package models

type CertificatePage struct {
	Items      []CertificateSummary `json:"items"`
	NextCursor *string              `json:"nextCursor"`
}
