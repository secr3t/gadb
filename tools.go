package gadb

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
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

		names := parseLaunchableActivityNames(stdout)
		if len(names) == 0 {
			log.Println(stdout)
			return "", errors.New(fmt.Sprintf("Unable to resolve the launchable activity of '%s'. Is it installed on the device?", pkg))
		}

		if len(names) == 1 {
			return names[0], nil
		}

		tmpRoot, err := ioutil.TempDir("", "tmpRoot")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpRoot)

		tmpApp, err := d.Pull(pkg, tmpRoot)
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

	stdout, stderr, err := m.shell([]string{"cmd", "package", "resolve-activity", "--brief", pkg})
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if m.isValidClass(line) {
			return line, nil
		}
	}

	return "", errors.New(fmt.Sprintf("Unable to resolve the launchable activity of '%s'. Original error: %s", pkg, stderr))
}
