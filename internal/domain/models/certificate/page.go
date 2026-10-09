package certificate

type CertificatePage struct {
	NextCursor *string              `json:"nextCursor"`
	Items      []CertificateSummary `json:"items"`
}
