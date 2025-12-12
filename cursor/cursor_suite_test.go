package cursor_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCursor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cursor Suite")
}
