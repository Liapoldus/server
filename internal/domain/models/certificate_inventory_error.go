package models

import "errors"

var ErrCertificateInventoryUnavailable = errors.New("certificate inventory unavailable")
var ErrCertificateNotFound = errors.New("certificate status not found")
var ErrInvalidCertificateQuery = errors.New("invalid certificate query")
