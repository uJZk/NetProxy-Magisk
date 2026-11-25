package module

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

const coreReleaseAPI = "https://api.github.com/repos/uJZk/NetProxy-Magisk/releases/tags/core-"

var coreAssetName = regexp.MustCompile(`^sing-box-(v[0-9]+\.[0-9]+\.[0-9]+(?:-(?:alpha|beta|rc)\.[0-9]+)?-reF1nd)-android-arm64$`)

func coreSlot(o Options, channel string) (string, error) {
	if channel != "stable" && channel != "testing" {
		return "", errors.New("请选择 stable 或 testing")
	}
	return filepath.Join(o.ModuleDir, "cores", channel), nil
}
func CoreStatus(ctx context.Context, o Options) (map[string]string, error) {
	result := map[string]string{}
	active, err := os.Stat(o.SingBoxPath)
	if err != nil {
		return nil, err
	}
	for _, slot := range []string{"active", "stable", "testing"} {
		path := o.SingBoxPath
		if slot != "active" {
			path, _ = coreSlot(o, slot)
		}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		if slot != "active" && os.SameFile(info, active) {
			result["active_channel"] = slot
		}
		output, err := exec.CommandContext(ctx, path, "version").Output()
		if err != nil {
			return nil, err
		}
		line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
		result[slot] = strings.TrimSpace(strings.TrimPrefix(line, "sing-box version "))
	}
	return result, nil
}

func fetchCore(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, fmt.Errorf("核心发布资源 HTTP %d", response.StatusCode)
	}
	return response, nil
}

func UpdateCore(ctx context.Context, o Options, channel string) error {
	dest, err := coreSlot(o, channel)
	if err != nil {
		return err
	}
	resp, err := fetchCore(ctx, coreReleaseAPI+channel)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var release struct {
		Name   string
		Assets []struct {
			Name, Digest string
			Size         int64
			URL          string `json:"browser_download_url"`
		}
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&release); err != nil {
		return err
	}
	for _, asset := range release.Assets {
		match := coreAssetName.FindStringSubmatch(asset.Name)
		if match == nil || match[1] != release.Name {
			continue
		}
		prerelease := strings.Count(match[1], "-") > 1
		if prerelease != (channel == "testing") {
			continue
		}
		expectedURL := "https://github.com/uJZk/NetProxy-Magisk/releases/download/core-" + channel + "/" + asset.Name
		if asset.URL != expectedURL || !strings.HasPrefix(asset.Digest, "sha256:") || len(asset.Digest) != 71 || asset.Size <= 0 || asset.Size > 192<<20 {
			return errors.New("核心发布元数据无效")
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		defer file.Close()
		download, err := fetchCore(ctx, expectedURL)
		if err != nil {
			return err
		}
		defer download.Body.Close()
		sum := sha256.New()
		size, err := io.Copy(io.MultiWriter(file, sum), io.LimitReader(download.Body, asset.Size+1))
		if err != nil {
			return err
		}
		if size != asset.Size || hex.EncodeToString(sum.Sum(nil)) != strings.TrimPrefix(asset.Digest, "sha256:") {
			return errors.New("核心大小或 SHA-256 不匹配")
		}
		ef, err := elf.NewFile(file)
		if err != nil {
			return err
		}
		if ef.Machine != elf.EM_AARCH64 || ef.Class != elf.ELFCLASS64 {
			return errors.New("核心架构不匹配")
		}
		if err = file.Chmod(0700); err != nil {
			return err
		}
		if err = file.Sync(); err != nil {
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		output, err := exec.CommandContext(ctx, file.Name(), "version").Output()
		if err != nil {
			return err
		}
		first, _, _ := strings.Cut(string(output), "\n")
		if strings.TrimPrefix(first, "sing-box version ") != strings.TrimPrefix(match[1], "v") {
			return errors.New("核心版本与发布标签不符")
		}
		lock, err := waitLifecycleLock(ctx, o.StateFile)
		if err != nil {
			return err
		}
		defer lock.release()
		// Keep the active inode accessible even when refreshing its channel.
		if old, statErr := os.Stat(dest); statErr == nil {
			active, activeErr := os.Stat(o.SingBoxPath)
			if activeErr != nil {
				return activeErr
			}
			if os.SameFile(old, active) {
				marker := filepath.Join(filepath.Dir(dest), "selected")
				if err := writeRawFile(marker, []byte(channel)); err != nil {
					return err
				}
			}
		}
		return os.Rename(file.Name(), dest)
	}
	return errors.New("渠道尚无可用的 Android arm64 核心")
}

func SwitchCore(ctx context.Context, o Options, channel string) error {
	slot, err := coreSlot(o, channel)
	if err != nil {
		return err
	}
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	fi, err := os.Stat(slot)
	if err != nil {
		return err
	}
	active, err := os.Stat(o.SingBoxPath)
	if err != nil {
		return err
	}
	if os.SameFile(active, fi) {
		return nil
	}
	_, source, _ := rawPaths(o)
	candidate := o
	candidate.SingBoxPath = slot
	if _, err = os.Stat(source); err == nil {
		if err = validateRawSite(ctx, candidate, source); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Keep both executable inodes until startup succeeds, including on a failed rename.
	dir, err := os.MkdirTemp(filepath.Dir(o.SingBoxPath), ".switch-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	old := filepath.Join(dir, "previous")
	next := filepath.Join(dir, "next")
	if err = os.Link(o.SingBoxPath, old); err != nil {
		return err
	}
	if err = os.Link(slot, next); err != nil {
		return err
	}
	state, err := ReadServiceState(o.StateFile)
	if err != nil {
		return err
	}
	running := service.FindProcess(o.SingBoxPath, int(state.PID)) > 0
	if running {
		if err = StopService(ctx, o); err != nil {
			return err
		}
	}
	if err = os.Rename(next, o.SingBoxPath); err == nil && running {
		err = StartService(ctx, o)
	}
	if err == nil {
		return nil
	}
	if restore := os.Rename(old, o.SingBoxPath); restore != nil {
		return errors.Join(err, restore)
	}
	if running {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		err = errors.Join(err, StartService(recovery, o))
	}
	return err
}
