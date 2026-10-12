package test_scenarios

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/features/catalog"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	reads "github.com/siti-nabila/grpc-auth/tests/features/catalog/mocks"
	"github.com/siti-nabila/grpc-auth/tests/features/role/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"testing"
	"time"
)

func Construction() []testutils.Scenario {
	return []testutils.Scenario{{Name: "role and catalog constructors fail before use with missing required dependency", Run: roleConstruction}}
}
func roleConstruction(t *testing.T) {
	snapshot := reads.NewSnapshot(context.Background())
	provider := mocks.Provider{BeginTransaction: func(context.Context) (domain.Transaction, error) {
		t.Fatal("constructor began transaction")
		return nil, nil
	}}
	for i := 0; i < 4; i++ {
		d := role.ManagementDependencies{Transactions: provider, Bind: role.BindManagementRepositories, Now: time.Now, EventID: func() (string, error) { return "", nil }}
		switch i {
		case 0:
			d.Transactions = nil
		case 1:
			d.Bind = nil
		case 2:
			d.Now = nil
		case 3:
			d.EventID = nil
		}
		if s, e := role.NewManagementService(d); s != nil || e == nil {
			t.Fatal("incomplete command dependencies accepted", i)
		}
	}
	for i := 0; i < 3; i++ {
		d := role.ReadDependencies{Snapshots: snapshot, Bind: role.BindReadRepositories, Now: time.Now}
		switch i {
		case 0:
			d.Snapshots = nil
		case 1:
			d.Bind = nil
		case 2:
			d.Now = nil
		}
		if s, e := role.NewReadService(d); s != nil || e == nil {
			t.Fatal("incomplete role read dependencies", i)
		}
		c := catalog.Dependencies{Snapshots: snapshot, Bind: catalog.BindRepositories, Now: time.Now}
		switch i {
		case 0:
			c.Snapshots = nil
		case 1:
			c.Bind = nil
		case 2:
			c.Now = nil
		}
		if s, e := catalog.NewService(c); s != nil || e == nil {
			t.Fatal("incomplete catalog dependencies", i)
		}
	}
}
