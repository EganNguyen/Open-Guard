package detector

import "context"

// Detector defines the common interface for all threat detection engines.
type Detector interface {
	Run(ctx context.Context) error
}
