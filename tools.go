package gadb

import (
	"errors"
	"fmt"
	"github.com/secr3t/gadb/utils"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Methods struct {
	EXEC_OUTPUT_FORMAT struct {
		FULL int
	}
}

func (d Device) GetApiLevel() (int, error) {
	// get API level logic (use adb shell getprop)
	output, err := d.RunShellCommand("getprop", "ro.build.version.sdk")
	if err != nil {
		return 0, err
	}
	apiLevel := strings.TrimSpace(output)
	// Convert API level string to int
	return strconv.Atoi(apiLevel)
}

func (d Device) packageAndLaunchActivityFromManifest(apk string) (string, error) {
	// Placeholder for logic to extract the main activity from the APK's manifest
	return "com.example.MainActivity", nil
}

func parseLaunchableActivityNames(stdout string) []string {
	// Logic to parse activity names from dumpsys output
	return []string{"com.example.MainActivity"}
}

func (d Device) isValidClass(className string) bool {
	// Logic to validate activity class name
	return strings.Contains(className, ".")
}

func (d Device) resolveLaunchableActivity(pkg string, opts map[string]interface{}) (string, error) {
	preferCmd := true
	if val, ok := opts["preferCmd"]; ok {
		preferCmd = val.(bool)
	}

	apiLevel, err := d.GetApiLevel()
	if err != nil {
		return "", err
	}

	if !preferCmd || apiLevel < 24 {
		stdout, err := d.RunShellCommand("dumpsys", "package", pkg)
		if err != nil {
			return "", err
		}

		names := utils.ParseLaunchableActivityNames(stdout)
		if len(names) == 0 {
			log.Println(stdout)
			return "", errors.New(fmt.Sprintf("Unable to resolve the launchable activity of '%s'. Is it installed on the device?", pkg))
		}

		if len(names) == 1 {
			return names[0], nil
		}

		tmpRoot, err := os.MkdirTemp("", "tmpRoot")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpRoot)

		tmpApp, err := d.pullApk(pkg, tmpRoot)
		if err != nil {
			return "", err
		}

		apkActivity, err := d.packageAndLaunchActivityFromManifest(tmpApp)
		if err != nil {
			log.Printf("Unable to resolve the launchable activity of '%s'. "+
				"The very first match of the dumpsys output is going to be used. "+
				"Original error: %s\n", pkg, err.Error())
			return names[0], nil
		}

		return apkActivity, nil
	}

	stdout, err := d.RunShellCommand("cmd", "package", "resolve-activity", "--brief", pkg)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if d.isValidClass(line) {
			return line, nil
		}
	}

	return "", errors.New(fmt.Sprintf("Unable to resolve the launchable activity of '%s'. Original error: %s", pkg, err))
}

func (d Device) pullApk(pkg string, tmpDir string) (string, error) {
	// 'pm path <pkg>' 명령어를 통해 APK 경로 얻기
	stdout, err := d.RunShellCommand("pm", "path", pkg)
	if err != nil {
		return "", err
	}

	packageMarker := "package:"
	if !strings.HasPrefix(stdout, packageMarker) {
		return "", errors.New(fmt.Sprintf("Cannot pull the .apk package for '%s'. Original error: %s", pkg, stdout))
	}

	// 원격 경로에서 패키지 부분을 제거
	remotePath := strings.Replace(stdout, packageMarker, "", 1)
	tmpApp := filepath.Join(tmpDir, fmt.Sprintf("%s.apk", pkg))
	tmpAppW, _ := os.Open(tmpApp)
	// 원격 경로에서 로컬 tmpDir로 APK 파일을 가져오기
	err = d.Pull(remotePath, tmpAppW)
	if err != nil {
		return "", err
	}

	fmt.Printf("Pulled app for package '%s' to '%s'\n", pkg, tmpApp)
	return tmpApp, nil
}
