package models

type CertificateSummary struct {
	Domain    string  `json:"domain"`
	Source    string  `json:"source"`
	Readiness string  `json:"readiness"`
	NotAfter  *string `json:"notAfter"`
	Serial    *string `json:"serial"`
}
