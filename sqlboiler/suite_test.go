package sqlboiler_test

import (
	"reflect"
	"testing"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSQLBoiler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQLBoiler Suite")
}

// modTypeName returns the type name of a query mod for assertion purposes.
func modTypeName(mod qm.QueryMod) string {
	return reflect.TypeOf(mod).String()
}

// whereModMatcher returns a Gomega matcher that matches any WHERE-type query mod.
func whereModMatcher() OmegaMatcher {
	return Or(
		Equal("qm.whereQueryMod"),
		Equal("qmhelper.WhereQueryMod"),
		Equal("qm.QueryModFunc"),
	)
}
