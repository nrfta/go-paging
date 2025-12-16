package offset_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOffset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Offset Suite")
}
