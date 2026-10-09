package interfaces

import (
	"context"

	certificatemodel "liapoldus.local/server-plugin/internal/domain/models/certificate"
)

type CertificateInventory interface {
	ListCertificates(context.Context, string, int, string) (certificatemodel.CertificatePage, error)
	CertificateStatus(context.Context, string) (certificatemodel.CertificateStatus, error)
}
