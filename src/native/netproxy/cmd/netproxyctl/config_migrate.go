package main

import (
	"os"
	"path/filepath"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

// migrateRawModule 转换指定文件；安装器应传入持锁复制的快照。
func migrateRawModule(modulePath, settingsPath string) error {
	content, err := os.ReadFile(modulePath)
	if err != nil {
		return err
	}
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		return err
	}
	migrated, err := moduleconfig.MigrateRawModule(content, settings)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(modulePath), ".module-migrate-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(migrated); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), modulePath)
}
