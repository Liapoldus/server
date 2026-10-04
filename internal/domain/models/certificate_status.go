package models

type CertificateStatus struct {
	Domain        string  `json:"domain"`
	Source        string  `json:"source"`
	Readiness     string  `json:"readiness"`
	NotBefore     *string `json:"notBefore"`
	NotAfter      *string `json:"notAfter"`
	Serial        *string `json:"serial"`
	LastErrorCode *string `json:"lastErrorCode"`
}
