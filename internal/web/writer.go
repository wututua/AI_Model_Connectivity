package web

import (
	"os"
	"path/filepath"
)

func EnsureAssets(webDir string) error {
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0755); err != nil {
		return err
	}
	files := map[string]string{
		filepath.Join(webDir, "index.html"):          indexHTML,
		filepath.Join(webDir, "assets", "app.js"):    appJS,
		filepath.Join(webDir, "assets", "style.css"): styleCSS,
	}
	for path, content := range files {
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}
