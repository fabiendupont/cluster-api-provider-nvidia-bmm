package controller

import (
	"context"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	restclient "github.com/NVIDIA/carbide-rest/client"
	"github.com/fabiendupont/cluster-api-provider-nvidia-bmm/internal/controller/testutil"
)

var _ = Describe("NvidiaBMMMachine Controller", func() {
	Context("When reconciling instance creation", func() {
		It("should create instance with correct parameters", func() {
			instanceID := uuid.New()
			mockClient := &testutil.MockCarbideClient{
				CreateInstanceFunc: func(ctx context.Context, org string, body restclient.CreateInstanceJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateInstanceResponse, error) {
					Expect(org).To(Equal("test-org"))
					Expect(body.Name).To(Equal("test-machine"))

					return &restclient.CreateInstanceResponse{
						HTTPResponse: testutil.MockHTTPResponse(201),
						JSON201: &restclient.Instance{
							Id:   &instanceID,
							Name: testutil.Ptr("test-machine"),
						},
					}, nil
				},
			}

			_ = mockClient
			// TODO: Implement full test with controller reconciliation
		})
	})
})
