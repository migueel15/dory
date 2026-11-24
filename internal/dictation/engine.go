package dictation

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// engineVersion is the NeMo-Speech.cpp release Dory downloads when the node
// has none (ADR-011). Only the CPU build, which runs on any machine.
const engineVersion = "0.3.0"

type engineAsset struct {
	File, SHA string
	Bytes     int64
}

var engineAssets = map[string]engineAsset{
	"linux/amd64":   {"linux-x86_64-cpu.tar.gz", "255f12b722ab15b2a38a51cf125d56e3fa97058b528f7e895b39d6a473deb06e", 9605101},
	"linux/arm64":   {"linux-aarch64-cpu.tar.gz", "56858eaf50d19b05902c043f65d0057352bdc107ae380c41053bfbddfc204bf1", 9064856},
	"darwin/arm64":  {"macos-aarch64-cpu.tar.gz", "2c3a7a719b778daeb14f147e011891f16e4b2ac62388d8b8d8b365cc2eebdf15", 6895976},
	"darwin/amd64":  {"macos-x86_64-cpu.tar.gz", "97e98aa5eb82dffa4f513f26336b5301d792c9fb43f720ac8f9520a0e92ffe3f", 7742012},
	"windows/amd64": {"windows-x86_64-cpu.zip", "4ed9740e361e4653f56c3dce7d1e81883a1c8b831c1bd7d01567fff1130b5b05", 8694509},
}

func platformAsset() (engineAsset, bool) {
	asset, ok := engineAssets[runtime.GOOS+"/"+runtime.GOARCH]
	return asset, ok
}

func (m *LocalModels) engineDir() string {
	return filepath.Join(m.dir, "runtime", engineVersion)
}

func (m *LocalModels) engineBin() string {
	name := "nemo-speech"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.engineDir(), "bin", name)
}

// binary finds NeMo on every use, so an engine installed while the node runs
// is picked up: the configured path, then the downloaded engine, then PATH.
func (m *LocalModels) binary() string {
	if m.bin != "" {
		path, _ := exec.LookPath(m.bin)
		return path
	}
	if info, err := os.Stat(m.engineBin()); err == nil && info.Mode().IsRegular() {
		return m.engineBin()
	}
	path, _ := exec.LookPath("nemo-speech")
	return path
}

// ensureEngine downloads the engine of this platform when none is found.
func (m *LocalModels) ensureEngine() error {
	m.engineMu.Lock()
	defer m.engineMu.Unlock()
	if m.binary() != "" {
		return nil
	}
	asset, ok := platformAsset()
	if !ok {
		return errors.New(i18n.Text("dictation.nemo_speech_no_publica"))
	}
	if err := os.MkdirAll(filepath.Dir(m.engineDir()), 0700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(m.dir, "nemo-speech-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(archive.Name())
	defer archive.Close()
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Minute)
	defer cancel()
	name := "nemo-speech-" + engineVersion + "-" + asset.File
	address := "https://github.com/NVIDIA/NeMo-Speech.cpp/releases/download/v" + engineVersion + "/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New(i18n.Text("dictation.no_pudo_descargar_nemo"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(i18n.Text("dictation.github_respondio_http_al"), resp.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, asset.Bytes+1))
	if err != nil {
		return err
	}
	if size != asset.Bytes || hex.EncodeToString(hash.Sum(nil)) != asset.SHA {
		return errors.New(i18n.Text("dictation.motor_descargado_no_coincide"))
	}
	staging, err := os.MkdirTemp(filepath.Dir(m.engineDir()), "extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if strings.HasSuffix(asset.File, ".zip") {
		err = extractZip(archive.Name(), staging)
	} else {
		err = extractTarGz(archive.Name(), staging)
	}
	if err != nil {
		return fmt.Errorf(i18n.Text("dictation.no_pudo_descomprimir_nemo"), err)
	}
	os.RemoveAll(m.engineDir())
	return os.Rename(staging, m.engineDir())
}

// entryPath drops the top folder of an archive entry and refuses paths that
// leave the destination.
func entryPath(dest, name string) (string, bool) {
	_, rest, ok := strings.Cut(strings.ReplaceAll(name, "\\", "/"), "/")
	if !ok || rest == "" {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(rest))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(dest, clean), true
}

func writeFile(path string, mode os.FileMode, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode&0755|0600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func extractTarGz(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		path, ok := entryPath(dest, h.Name)
		if !ok {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0700)
		case tar.TypeReg:
			err = writeFile(path, os.FileMode(h.Mode), tr)
		case tar.TypeSymlink:
			// Library aliases point to a sibling file.
			if strings.ContainsAny(h.Linkname, `/\`) {
				return errors.New("enlace fuera de su carpeta: " + h.Name)
			}
			if err = os.MkdirAll(filepath.Dir(path), 0700); err == nil {
				err = os.Symlink(h.Linkname, path)
			}
		}
		if err != nil {
			return err
		}
	}
}

func extractZip(archive, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, entry := range zr.File {
		path, ok := entryPath(dest, entry.Name)
		if !ok || entry.FileInfo().IsDir() {
			continue
		}
		r, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeFile(path, 0755, r)
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
