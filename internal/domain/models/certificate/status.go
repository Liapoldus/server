package certificate

type CertificateStatus struct {
	NotBefore     *string `json:"notBefore"`
	NotAfter      *string `json:"notAfter"`
	Serial        *string `json:"serial"`
	LastErrorCode *string `json:"lastErrorCode"`
	Domain        string  `json:"domain"`
	Source        string  `json:"source"`
	Readiness     string  `json:"readiness"`
}
