//go:build xshop_production

package xshopupgrade

import (
	"strings"
	"testing"
)

func TestProductionOperatorPolicyFixedDatabasePort(t *testing.T) {
	p := OperatorPolicy{PublicKey: strings.Repeat("01", 32), Schema: strings.Repeat("a", 64), Database: "/opt/dujiao-next/db/dujiao.db", Port: 18080, BootstrapSequence: 1}
	if err := ValidateOperatorPolicy(p); err != nil {
		t.Fatal(err)
	}
	for _, db := range []string{"/opt/dujiao-preview/db/dujiao.db", "/opt/dujiao-next/db/other.db", "/opt/dujiao-next/db/../db/dujiao.db"} {
		q := p
		q.Database = db
		if ValidateOperatorPolicy(q) == nil {
			t.Fatal("wrong database", db)
		}
	}
	q := p
	q.Port = 18083
	if ValidateOperatorPolicy(q) == nil {
		t.Fatal("preview port accepted")
	}
}
