package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// RunWorker is only invoked by the separate root-owned systemd unit. No HTTP
// parameter controls executable paths, environment variables or download URLs.
func RunWorker() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return ErrUnsupported
	}
	for _, path := range []string{StateDir, StateDir + "/enabled", HelperDir, HelperDir + "/install.sh", InstallDir,
		InstallDir + "/.installer.json", "/etc/systemd/system/model-connectivity.service"} {
		if !trustedPath(path) {
			return fmt.Errorf("untrusted updater path: %s", path)
		}
	}
	lock, err := workerLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	enabled, err := os.ReadFile(StateDir + "/enabled")
	if err != nil || string(enabled) != "1\n" {
		return ErrUnsupported
	}
	client := New("", "")
	return runWorker(StateDir, func(ctx context.Context, channel string) (Check, error) {
		return client.Check(ctx, channel)
	}, func() (string, error) {
		var state struct {
			Version string `json:"version"`
		}
		err := readJSON(InstallDir+"/.installer.json", &state, 8192)
		return state.Version, err
	}, runInstaller)
}

type installerRun func(version string, progress func(string) error) error

func runWorker(dir string, check func(context.Context, string) (Check, error), current func() (string, error), run installerRun) error {
	path := filepath.Join(dir, "requests", "request.json")
	var previous Job
	if err := readJSON(filepath.Join(dir, "status.json"), &previous, 8192); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if previous.Active() {
		return ErrBusy
	}
	var job Job
	if err := readJSON(path, &job, 4096); err != nil {
		return err
	}
	if !idPattern.MatchString(job.ID) || !validTag(job.Version) || !validChannel(job.Channel) {
		// Removing a directory entry never follows a symlink supplied by the app.
		_ = os.Remove(path)
		return errors.New("invalid update request")
	}
	job.Status, job.Stage, job.Message = "running", "checking", "正在核对目标版本"
	save := func() error {
		job.UpdatedAt = time.Now().UTC()
		return atomicJSON(filepath.Join(dir, "status.json"), job, 0644)
	}
	if err := save(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	// The request is consumed before stopping the application. A power loss must
	// leave an unresolved task, never cause an unattended second installation.
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	installed, err := current()
	if err == nil && (!validTag(installed) || semver.Compare(job.Version, installed) <= 0) {
		err = ErrVersion
	}
	if err == nil {
		var checked Check
		checked, err = check(ctx, job.Channel)
		if err == nil && (checked.Release == nil || checked.Release.Version != job.Version || !checked.Release.PackageAvailable) {
			err = ErrVersion
		}
	}
	restored := false
	if err == nil {
		err = run(job.Version, func(stage string) error {
			if !validStage(stage) {
				return nil
			}
			job.Stage = stage
			if stage == "restored" {
				restored = true
			}
			return save()
		})
	}
	switch {
	case err == nil:
		job.Message = "更新完成，原服务运行状态已保留"
		if job.Stage == "restarting" {
			job.Message = "更新完成，服务已通过本机健康检查"
		}
		job.Status, job.Stage = "succeeded", "complete"
	case restored:
		job.Status, job.Stage, job.Message = "rolled_back", "restored", "更新失败，已恢复之前的程序、数据库和服务"
	default:
		job.Status, job.Message = "failed", "更新未完成。请查看独立更新服务日志并确认服务状态，勿直接重复更新。"
		if job.Stage == "installing" || job.Stage == "restarting" || job.Stage == "restoring" {
			job.Status = "recovery_required"
		}
	}
	saveErr := save()
	if err != nil {
		return err
	}
	return saveErr
}

func validStage(stage string) bool {
	switch stage {
	case "downloading", "verifying", "backup", "installing", "restarting", "restoring", "restored":
		return true
	}
	return false
}

func runInstaller(version string, progress func(string) error) error {
	command := exec.Command("/bin/bash", HelperDir+"/install.sh", "upgrade", "--version", version, "--yes")
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "CG_UPDATE_PROGRESS=1"}
	command.Stdin = nil
	command.Stderr = os.Stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var progressErr error
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		if stage, ok := strings.CutPrefix(line, "CG_UPDATE_STAGE="); ok {
			if err := progress(stage); err != nil {
				progressErr = err
			}
		}
	}
	// Keep draining on oversized output so the installer can finish or roll back.
	if scanner.Err() != nil {
		_, _ = io.Copy(io.Discard, stdout)
	}
	err = command.Wait()
	if err != nil {
		return err
	}
	return errors.Join(progressErr, scanner.Err())
}
