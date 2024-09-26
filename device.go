package gadb

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var DeviceTempPath = "/data/local/tmp"

type DeviceFileInfo struct {
	Name         string
	Mode         os.FileMode
	Size         uint32
	LastModified time.Time
}

func (info DeviceFileInfo) IsDir() bool {
	return (info.Mode & (1 << 14)) == (1 << 14)
}

const DefaultFileMode = os.FileMode(0664)

type DeviceState string

const (
	StateUnknown      DeviceState = "UNKNOWN"
	StateOnline       DeviceState = "online"
	StateOffline      DeviceState = "offline"
	StateUnauthorized DeviceState = "unauthorized"
	StateDisconnected DeviceState = "disconnected"
)

var deviceStateStrings = map[string]DeviceState{
	"":             StateDisconnected,
	"offline":      StateOffline,
	"device":       StateOnline,
	"unauthorized": StateUnauthorized,
}

func deviceStateConv(k string) (deviceState DeviceState) {
	var ok bool
	if deviceState, ok = deviceStateStrings[k]; !ok {
		return StateUnknown
	}
	return
}

type DeviceForward struct {
	Serial string
	Local  string
	Remote string
	// LocalProtocol string
	// RemoteProtocol string
}

type Device struct {
	adbClient    Client
	serial       string
	attrs        map[string]string
	IsAuthorized bool
}

func (d Device) Product() string {
	return d.attrs["product"]
}

func (d Device) Model() string {
	return d.attrs["model"]
}

func (d Device) Usb() string {
	return d.attrs["usb"]
}

func (d Device) transportId() string {
	return d.attrs["transport_id"]
}

func (d Device) DeviceInfo() map[string]string {
	return d.attrs
}

func (d Device) Serial() string {
	// 	resp, err := d.adbClient.executeCommand(fmt.Sprintf("host-serial:%s:get-serialno", d.serial))
	return d.serial
}

func (d Device) IsUsb() bool {
	return d.Usb() != ""
}

func (d Device) State() (DeviceState, error) {
	resp, err := d.adbClient.executeCommand(fmt.Sprintf("host-serial:%s:get-state", d.serial))
	return deviceStateConv(resp), err
}

func (d Device) DevicePath() (string, error) {
	resp, err := d.adbClient.executeCommand(fmt.Sprintf("host-serial:%s:get-devpath", d.serial))
	return resp, err
}

func (d Device) Forward(localPort, remotePort int, noRebind ...bool) (err error) {
	command := ""
	local := fmt.Sprintf("tcp:%d", localPort)
	remote := fmt.Sprintf("tcp:%d", remotePort)

	if len(noRebind) != 0 && noRebind[0] {
		command = fmt.Sprintf("host-serial:%s:forward:norebind:%s;%s", d.serial, local, remote)
	} else {
		command = fmt.Sprintf("host-serial:%s:forward:%s;%s", d.serial, local, remote)
	}

	_, err = d.adbClient.executeCommand(command, true)
	return
}

func (d Device) ForwardList() (deviceForwardList []DeviceForward, err error) {
	var forwardList []DeviceForward
	if forwardList, err = d.adbClient.ForwardList(); err != nil {
		return nil, err
	}

	deviceForwardList = make([]DeviceForward, 0, len(deviceForwardList))
	for i := range forwardList {
		if forwardList[i].Serial == d.serial {
			deviceForwardList = append(deviceForwardList, forwardList[i])
		}
	}
	// resp, err := d.adbClient.executeCommand(fmt.Sprintf("host-serial:%s:list-forward", d.serial))
	return
}

func (d Device) ChangeIp() {
	d.DisableData()
	time.Sleep(5 * time.Second)
	d.EnableData()
}

func (d Device) IsLocked() bool {
	result, _ := d.RunShellCommand("dumpsys power | grep 'Display Power: state='")
	powerState := strings.Split(result, "state=")

	return strings.ToUpper(strings.TrimSpace(powerState[1])) != "ON"
}

func (d Device) Lock() {
	if d.IsLocked() {
		return
	}
	d.ToggleScreenLock()
}

func (d Device) Unlock() {
	if !d.IsLocked() {
		return
	}
	d.ToggleScreenLock()
}

func (d Device) DisableScreenRotation() {
	d.RunShellCommand("settings put system accelerometer_rotation 0")
}

func (d Device) EnableScreenRotation() {
	d.RunShellCommand("settings put system accelerometer_rotation 1")
}

func (d Device) ToggleScreenLock() {
	d.RunShellCommand("input keyevent 26")
}

func WaitFor(predicate func() bool) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for range ticker.C {
		if predicate() {
			ticker.Stop()
			break
		}
	}
}

func (d Device) IsNetworkUnAvailable() bool {
	return !d.IsNetworkAvailable()
}
func (d Device) IsNetworkAvailable() bool {
	return strings.Contains(d.pingGoogle(), "1 packets transmitted")
}

func (d Device) pingGoogle() (ip string) {
	// ping -c 1 google.com
	// ping: unknown host google.com --> error
	ip, _ = d.RunShellCommand("ping", "-c", "1", "google.com")
	return
}

func (d Device) DisableData() {
	d.RunShellCommand("svc", "data", "disable")
}

func (d Device) EnableData() {
	d.RunShellCommand("svc", "data", "enable")
}

func (d Device) ForwardKill(localPort int) (err error) {
	local := fmt.Sprintf("tcp:%d", localPort)
	_, err = d.adbClient.executeCommand(fmt.Sprintf("host-serial:%s:killforward:%s", d.serial, local), true)
	return
}

func (d Device) RunShellCommand(cmd string, args ...string) (string, error) {
	raw, err := d.RunShellCommandWithBytes(cmd, args...)
	return string(raw), err
}

func (d Device) RunShellCommandWithBytes(cmd string, args ...string) ([]byte, error) {
	if len(args) > 0 {
		cmd = fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
	}
	if strings.TrimSpace(cmd) == "" {
		return nil, errors.New("adb shell: command cannot be empty")
	}
	raw, err := d.executeCommand(fmt.Sprintf("shell:%s", cmd))
	return raw, err
}

func (d Device) EnableAdbOverTCP(port ...int) (err error) {
	if len(port) == 0 {
		port = []int{AdbDaemonPort}
	}

	_, err = d.executeCommand(fmt.Sprintf("tcpip:%d", port[0]), true)
	return
}

func (d Device) createDeviceTransport() (tp transport, err error) {
	if tp, err = newTransport(fmt.Sprintf("%s:%d", d.adbClient.host, d.adbClient.port)); err != nil {
		return transport{}, err
	}

	if err = tp.Send(fmt.Sprintf("host:transport:%s", d.serial)); err != nil {
		return transport{}, err
	}
	err = tp.VerifyResponse()
	return
}

func (d Device) executeCommand(command string, onlyVerifyResponse ...bool) (raw []byte, err error) {
	if len(onlyVerifyResponse) == 0 {
		onlyVerifyResponse = []bool{false}
	}

	var tp transport
	if tp, err = d.createDeviceTransport(); err != nil {
		return nil, err
	}
	defer func() { _ = tp.Close() }()

	if err = tp.Send(command); err != nil {
		return nil, err
	}

	if err = tp.VerifyResponse(); err != nil {
		return nil, err
	}

	if onlyVerifyResponse[0] {
		return
	}

	raw, err = tp.ReadBytesAll()
	return
}

func (d Device) List(remotePath string) (devFileInfos []DeviceFileInfo, err error) {
	var tp transport
	if tp, err = d.createDeviceTransport(); err != nil {
		return nil, err
	}
	defer func() { _ = tp.Close() }()

	var sync syncTransport
	if sync, err = tp.CreateSyncTransport(); err != nil {
		return nil, err
	}
	defer func() { _ = sync.Close() }()

	if err = sync.Send("LIST", remotePath); err != nil {
		return nil, err
	}

	devFileInfos = make([]DeviceFileInfo, 0)

	var entry DeviceFileInfo
	for entry, err = sync.ReadDirectoryEntry(); err == nil; entry, err = sync.ReadDirectoryEntry() {
		if entry == (DeviceFileInfo{}) {
			break
		}
		devFileInfos = append(devFileInfos, entry)
	}

	return
}

func (d Device) PushFile(local *os.File, remotePath string, modification ...time.Time) (err error) {
	if len(modification) == 0 {
		var stat os.FileInfo
		if stat, err = local.Stat(); err != nil {
			return err
		}
		modification = []time.Time{stat.ModTime()}
	}

	return d.Push(local, remotePath, modification[0], DefaultFileMode)
}

func (d Device) Push(source io.Reader, remotePath string, modification time.Time, mode ...os.FileMode) (err error) {
	if len(mode) == 0 {
		mode = []os.FileMode{DefaultFileMode}
	}

	var tp transport
	if tp, err = d.createDeviceTransport(); err != nil {
		return err
	}
	defer func() { _ = tp.Close() }()

	var sync syncTransport
	if sync, err = tp.CreateSyncTransport(); err != nil {
		return err
	}
	defer func() { _ = sync.Close() }()

	data := fmt.Sprintf("%s,%d", remotePath, mode[0])
	if err = sync.Send("SEND", data); err != nil {
		return err
	}

	if err = sync.SendStream(source); err != nil {
		return
	}

	if err = sync.SendStatus("DONE", uint32(modification.Unix())); err != nil {
		return
	}

	if err = sync.VerifyStatus(); err != nil {
		return
	}
	return
}

func (d Device) Pull(remotePath string, dest io.Writer) (err error) {
	var tp transport
	if tp, err = d.createDeviceTransport(); err != nil {
		return err
	}
	defer func() { _ = tp.Close() }()

	var sync syncTransport
	if sync, err = tp.CreateSyncTransport(); err != nil {
		return err
	}
	defer func() { _ = sync.Close() }()

	if err = sync.Send("RECV", remotePath); err != nil {
		return err
	}

	err = sync.WriteStream(dest)
	return
}

func (d Device) InstallApk(apkPath string, reinstall ...bool) (err error) {

	apkName := filepath.Base(apkPath)
	if !strings.HasSuffix(strings.ToLower(apkName), ".apk") {
		return fmt.Errorf("apk file must have an extension of '.apk': %s", apkPath)
	}

	var apkFile *os.File
	if apkFile, err = os.Open(apkPath); err != nil {
		return fmt.Errorf("apk file: %w", err)
	}

	remotePath := path.Join(DeviceTempPath, apkName)
	if err = d.PushFile(apkFile, remotePath); err != nil {
		return fmt.Errorf("apk push: %w", err)
	}

	var shellOutput string
	if len(reinstall) != 0 && reinstall[0] {
		shellOutput, err = d.RunShellCommand("pm install", "-r", remotePath)
	} else {
		shellOutput, err = d.RunShellCommand("pm install", remotePath)
	}

	if err != nil {
		return fmt.Errorf("apk install: %w", err)
	}

	if !strings.Contains(shellOutput, "Success") {
		return fmt.Errorf("apk installed: %s", shellOutput)
	}

	return
}

func (d Device) AppUninstall(appPackageName string, keepDataAndCache ...bool) (err error) {
	var shellOutput string
	if len(keepDataAndCache) != 0 && keepDataAndCache[0] {
		shellOutput, err = d.RunShellCommand("pm uninstall", "-k", appPackageName)
	} else {
		shellOutput, err = d.RunShellCommand("pm uninstall", appPackageName)
	}

	if err != nil {
		return fmt.Errorf("apk uninstall: %w", err)
	}

	if !strings.Contains(shellOutput, "Success") {
		return fmt.Errorf("apk uninstalled: %s", shellOutput)
	}

	return
}
