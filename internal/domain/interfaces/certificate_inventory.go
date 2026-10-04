package interfaces

import (
	"context"

	"liapoldus.local/server-plugin/internal/domain/models"
)

type CertificateInventory interface {
	ListCertificates(context.Context, string, int, string) (models.CertificatePage, error)
	CertificateStatus(context.Context, string) (models.CertificateStatus, error)
}
