package paging_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGoPaging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GoPaging Test Suite")
}
