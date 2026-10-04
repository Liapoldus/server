package contracts

import (
	"encoding/json"
	"io/fs"
)

type SecretPurposes struct {
	ServerCertificate string `json:"serverCertificate"`
	ServerPrivateKey  string `json:"serverPrivateKey"`
	UpstreamCA        string `json:"upstreamCA"`
}

func LoadSecretPurposes() (SecretPurposes, error) {
	contents, err := fs.ReadFile(files, "v1/secret-purposes.json")
	if err != nil {
		return SecretPurposes{}, ErrInvalidAssets
	}
	var purposes SecretPurposes
	if err := json.Unmarshal(contents, &purposes); err != nil || purposes.ServerCertificate == "" ||
		purposes.ServerPrivateKey == "" || purposes.UpstreamCA == "" ||
		purposes.ServerCertificate == purposes.ServerPrivateKey || purposes.ServerCertificate == purposes.UpstreamCA ||
		purposes.ServerPrivateKey == purposes.UpstreamCA {
		return SecretPurposes{}, ErrInvalidAssets
	}
	return purposes, nil
}
