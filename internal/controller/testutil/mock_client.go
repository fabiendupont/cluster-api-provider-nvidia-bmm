package testutil

import (
	"context"
	"net/http"

	restclient "github.com/NVIDIA/carbide-rest/client"
	"github.com/google/uuid"
)

// MockCarbideClient is a mock implementation of ClientWithResponses for testing
type MockCarbideClient struct {
	// VPC methods
	CreateVPCFunc func(ctx context.Context, org string, body restclient.CreateVpcJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateVpcResponse, error)
	GetVPCFunc    func(ctx context.Context, org string, vpcId uuid.UUID, params *restclient.GetVpcParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetVpcResponse, error)
	DeleteVPCFunc func(ctx context.Context, org string, vpcId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteVpcResponse, error)

	// Subnet methods
	CreateSubnetFunc func(ctx context.Context, org string, body restclient.CreateSubnetJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateSubnetResponse, error)
	GetSubnetFunc    func(ctx context.Context, org string, subnetId uuid.UUID, params *restclient.GetSubnetParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetSubnetResponse, error)
	DeleteSubnetFunc func(ctx context.Context, org string, subnetId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteSubnetResponse, error)

	// Instance methods
	CreateInstanceFunc func(ctx context.Context, org string, body restclient.CreateInstanceJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateInstanceResponse, error)
	GetInstanceFunc    func(ctx context.Context, org string, instanceId uuid.UUID, params *restclient.GetInstanceParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetInstanceResponse, error)
	DeleteInstanceFunc func(ctx context.Context, org string, instanceId uuid.UUID, body restclient.DeleteInstanceJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteInstanceResponse, error)

	// Network Security Group methods
	CreateNetworkSecurityGroupFunc func(ctx context.Context, org string, body restclient.CreateNetworkSecurityGroupJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateNetworkSecurityGroupResponse, error)
	GetNetworkSecurityGroupFunc    func(ctx context.Context, org string, nsgId uuid.UUID, params *restclient.GetNetworkSecurityGroupParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetNetworkSecurityGroupResponse, error)
	DeleteNetworkSecurityGroupFunc func(ctx context.Context, org string, nsgId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteNetworkSecurityGroupResponse, error)

	// IP Block methods
	CreateIpblockFunc func(ctx context.Context, org string, body restclient.CreateIpblockJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateIpblockResponse, error)
	GetIpblockFunc    func(ctx context.Context, org string, ipBlockId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.GetIpblockResponse, error)
	DeleteIpblockFunc func(ctx context.Context, org string, ipBlockId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteIpblockResponse, error)
}

// Implement VPC methods
func (m *MockCarbideClient) CreateVPCWithResponse(ctx context.Context, org string, body restclient.CreateVpcJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateVpcResponse, error) {
	if m.CreateVPCFunc != nil {
		return m.CreateVPCFunc(ctx, org, body, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) GetVPCWithResponse(ctx context.Context, org string, vpcId uuid.UUID, params *restclient.GetVpcParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetVpcResponse, error) {
	if m.GetVPCFunc != nil {
		return m.GetVPCFunc(ctx, org, vpcId, params, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) DeleteVPCWithResponse(ctx context.Context, org string, vpcId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteVpcResponse, error) {
	if m.DeleteVPCFunc != nil {
		return m.DeleteVPCFunc(ctx, org, vpcId, reqEditors...)
	}
	return nil, nil
}

// Implement Subnet methods
func (m *MockCarbideClient) CreateSubnetWithResponse(ctx context.Context, org string, body restclient.CreateSubnetJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateSubnetResponse, error) {
	if m.CreateSubnetFunc != nil {
		return m.CreateSubnetFunc(ctx, org, body, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) GetSubnetWithResponse(ctx context.Context, org string, subnetId uuid.UUID, params *restclient.GetSubnetParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetSubnetResponse, error) {
	if m.GetSubnetFunc != nil {
		return m.GetSubnetFunc(ctx, org, subnetId, params, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) DeleteSubnetWithResponse(ctx context.Context, org string, subnetId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteSubnetResponse, error) {
	if m.DeleteSubnetFunc != nil {
		return m.DeleteSubnetFunc(ctx, org, subnetId, reqEditors...)
	}
	return nil, nil
}

// Implement Instance methods
func (m *MockCarbideClient) CreateInstanceWithResponse(ctx context.Context, org string, body restclient.CreateInstanceJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateInstanceResponse, error) {
	if m.CreateInstanceFunc != nil {
		return m.CreateInstanceFunc(ctx, org, body, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) GetInstanceWithResponse(ctx context.Context, org string, instanceId uuid.UUID, params *restclient.GetInstanceParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetInstanceResponse, error) {
	if m.GetInstanceFunc != nil {
		return m.GetInstanceFunc(ctx, org, instanceId, params, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) DeleteInstanceWithResponse(ctx context.Context, org string, instanceId uuid.UUID, body restclient.DeleteInstanceJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteInstanceResponse, error) {
	if m.DeleteInstanceFunc != nil {
		return m.DeleteInstanceFunc(ctx, org, instanceId, body, reqEditors...)
	}
	return nil, nil
}

// Implement NetworkSecurityGroup methods
func (m *MockCarbideClient) CreateNetworkSecurityGroupWithResponse(ctx context.Context, org string, body restclient.CreateNetworkSecurityGroupJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateNetworkSecurityGroupResponse, error) {
	if m.CreateNetworkSecurityGroupFunc != nil {
		return m.CreateNetworkSecurityGroupFunc(ctx, org, body, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) GetNetworkSecurityGroupWithResponse(ctx context.Context, org string, nsgId uuid.UUID, params *restclient.GetNetworkSecurityGroupParams, reqEditors ...restclient.RequestEditorFn) (*restclient.GetNetworkSecurityGroupResponse, error) {
	if m.GetNetworkSecurityGroupFunc != nil {
		return m.GetNetworkSecurityGroupFunc(ctx, org, nsgId, params, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) DeleteNetworkSecurityGroupWithResponse(ctx context.Context, org string, nsgId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteNetworkSecurityGroupResponse, error) {
	if m.DeleteNetworkSecurityGroupFunc != nil {
		return m.DeleteNetworkSecurityGroupFunc(ctx, org, nsgId, reqEditors...)
	}
	return nil, nil
}

// Implement IPBlock methods
func (m *MockCarbideClient) CreateIpblockWithResponse(ctx context.Context, org string, body restclient.CreateIpblockJSONRequestBody, reqEditors ...restclient.RequestEditorFn) (*restclient.CreateIpblockResponse, error) {
	if m.CreateIpblockFunc != nil {
		return m.CreateIpblockFunc(ctx, org, body, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) GetIpblockWithResponse(ctx context.Context, org string, ipBlockId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.GetIpblockResponse, error) {
	if m.GetIpblockFunc != nil {
		return m.GetIpblockFunc(ctx, org, ipBlockId, reqEditors...)
	}
	return nil, nil
}

func (m *MockCarbideClient) DeleteIpblockWithResponse(ctx context.Context, org string, ipBlockId uuid.UUID, reqEditors ...restclient.RequestEditorFn) (*restclient.DeleteIpblockResponse, error) {
	if m.DeleteIpblockFunc != nil {
		return m.DeleteIpblockFunc(ctx, org, ipBlockId, reqEditors...)
	}
	return nil, nil
}

// Helper functions to create common response objects

func MockHTTPResponse(statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
	}
}

func Ptr[T any](v T) *T {
	return &v
}
