package contracts

import (
	"encoding/json"
	"io/fs"
)

func PluginManifest() ([]byte, error) {
	contents, err := fs.ReadFile(files, "v1/plugin.json")
	if err != nil || !json.Valid(contents) {
		return nil, ErrInvalidAssets
	}
	return contents, nil
}
