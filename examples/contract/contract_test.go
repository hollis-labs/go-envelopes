package contract

import (
	"context"
	"testing"

	envelopes "github.com/hollis-labs/go-envelopes"
	"github.com/hollis-labs/go-envelopes/envelopestest"
)

// TestEnvelopesContract pins the public envelope-protocol contract.
// A host typically runs this against the registry it actually uses
// at runtime — including any plugin types — so that catalog drift
// is caught the moment the suite runs.
func TestEnvelopesContract(t *testing.T) {
	reg, err := envelopes.LoadCore(context.Background())
	if err != nil {
		t.Fatalf("load core registry: %v", err)
	}
	envelopestest.RunContract(t, reg)
}
