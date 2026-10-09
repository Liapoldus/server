package certificate

type CertificateSummary struct {
	NotAfter  *string `json:"notAfter"`
	Serial    *string `json:"serial"`
	Domain    string  `json:"domain"`
	Source    string  `json:"source"`
	Readiness string  `json:"readiness"`
}
