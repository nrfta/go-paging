package paging_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	ctx       context.Context
	container *Container
)

var _ = BeforeSuite(func() {
	ctx = context.Background()
	var err error

	// Start PostgreSQL container
	container, err = SetupPostgres(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(container).ToNot(BeNil())
	Expect(container.DB).ToNot(BeNil())

	GinkgoWriter.Printf("PostgreSQL container started: %s\n", container.ConnStr)
})

var _ = AfterSuite(func() {
	if container != nil {
		err := container.Terminate(ctx)
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Println("PostgreSQL container terminated")
	}
})

func TestPagingIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Paging Integration Suite")
}
