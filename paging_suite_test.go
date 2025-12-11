package paging_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPaging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Paging Suite")
}
