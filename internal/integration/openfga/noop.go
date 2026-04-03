package openfga

import (
    "context"
    "log/slog"

    openfga "github.com/openfga/go-sdk"
    "github.com/openfga/go-sdk/client"
)

// NoopClient implements the ClientInterface interface as a no-op for development/testing
type NoopClient struct {
    logger *slog.Logger
}

func (c *NoopClient) ListStores(ctx context.Context) client.SdkClientListStoresRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListStoresExecute(request client.SdkClientListStoresRequestInterface) (*client.ClientListStoresResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) CreateStore(ctx context.Context) client.SdkClientCreateStoreRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) CreateStoreExecute(request client.SdkClientCreateStoreRequestInterface) (*client.ClientCreateStoreResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) GetStore(ctx context.Context) client.SdkClientGetStoreRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) GetStoreExecute(request client.SdkClientGetStoreRequestInterface) (*client.ClientGetStoreResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) DeleteStore(ctx context.Context) client.SdkClientDeleteStoreRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) DeleteStoreExecute(request client.SdkClientDeleteStoreRequestInterface) (*client.ClientDeleteStoreResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadAuthorizationModels(ctx context.Context) client.SdkClientReadAuthorizationModelsRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadAuthorizationModelsExecute(request client.SdkClientReadAuthorizationModelsRequestInterface) (*client.ClientReadAuthorizationModelsResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteAuthorizationModel(ctx context.Context) client.SdkClientWriteAuthorizationModelRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteAuthorizationModelExecute(request client.SdkClientWriteAuthorizationModelRequestInterface) (*client.ClientWriteAuthorizationModelResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadAuthorizationModel(ctx context.Context) client.SdkClientReadAuthorizationModelRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadAuthorizationModelExecute(request client.SdkClientReadAuthorizationModelRequestInterface) (*client.ClientReadAuthorizationModelResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadLatestAuthorizationModel(ctx context.Context) client.SdkClientReadLatestAuthorizationModelRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadLatestAuthorizationModelExecute(request client.SdkClientReadLatestAuthorizationModelRequestInterface) (*client.ClientReadAuthorizationModelResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadChanges(ctx context.Context) client.SdkClientReadChangesRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadChangesExecute(request client.SdkClientReadChangesRequestInterface) (*client.ClientReadChangesResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) Read(ctx context.Context) client.SdkClientReadRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadExecute(request client.SdkClientReadRequestInterface) (*client.ClientReadResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) Write(ctx context.Context) client.SdkClientWriteRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteExecute(request client.SdkClientWriteRequestInterface) (*client.ClientWriteResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteTuples(ctx context.Context) client.SdkClientWriteTuplesRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteTuplesExecute(request client.SdkClientWriteTuplesRequestInterface) (*client.ClientWriteResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) DeleteTuples(ctx context.Context) client.SdkClientDeleteTuplesRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) DeleteTuplesExecute(request client.SdkClientDeleteTuplesRequestInterface) (*client.ClientWriteResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) Check(ctx context.Context) client.SdkClientCheckRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) CheckExecute(request client.SdkClientCheckRequestInterface) (*client.ClientCheckResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ClientBatchCheck(ctx context.Context) client.SdkClientBatchCheckClientRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ClientBatchCheckExecute(request client.SdkClientBatchCheckClientRequestInterface) (*client.ClientBatchCheckClientResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) BatchCheck(ctx context.Context) client.SdkClientBatchCheckRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) BatchCheckExecute(request client.SdkClientBatchCheckRequestInterface) (*openfga.BatchCheckResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) Expand(ctx context.Context) client.SdkClientExpandRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ExpandExecute(request client.SdkClientExpandRequestInterface) (*client.ClientExpandResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListObjects(ctx context.Context) client.SdkClientListObjectsRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListObjectsExecute(request client.SdkClientListObjectsRequestInterface) (*client.ClientListObjectsResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListRelations(ctx context.Context) client.SdkClientListRelationsRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListRelationsExecute(request client.SdkClientListRelationsRequestInterface) (*client.ClientListRelationsResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListUsers(ctx context.Context) client.SdkClientListUsersRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ListUsersExecute(r client.SdkClientListUsersRequestInterface) (*client.ClientListUsersResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) StreamedListObjects(ctx context.Context) client.SdkClientStreamedListObjectsRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) StreamedListObjectsExecute(request client.SdkClientStreamedListObjectsRequestInterface) (*client.ClientStreamedListObjectsResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadAssertions(ctx context.Context) client.SdkClientReadAssertionsRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) ReadAssertionsExecute(request client.SdkClientReadAssertionsRequestInterface) (*client.ClientReadAssertionsResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteAssertions(ctx context.Context) client.SdkClientWriteAssertionsRequestInterface {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) WriteAssertionsExecute(request client.SdkClientWriteAssertionsRequestInterface) (*client.ClientWriteAssertionsResponse, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) SetAuthorizationModelId(authorizationModelId string) error {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) GetAuthorizationModelId() (string, error) {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) SetStoreId(storeId string) error {
    // TODO implement me
    panic("implement me")
}

func (c *NoopClient) GetStoreId() (string, error) {
    // TODO implement me
    panic("implement me")
}

// Compile-time check to ensure NoopClient implements the ClientInterface interface
// var _ ClientInterface = (*NoopClient)(nil)
var _ OpenFGAClientInterface = (*NoopClient)(nil)

// NewNoopClient creates a new no-op OpenFGA client
func NewNoopClient(logger *slog.Logger) *NoopClient {
    return &NoopClient{
        logger: logger,
    }
}

/*// Check performs a no-op authorization check (always returns allowed)
func (c *NoopClient) Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
    c.logger.Debug("NoopClient: Check called", "user", req.User, "relation", req.Relation, "object", req.Object)
    return &CheckResponse{Allowed: true}, nil
}

// BatchCheck performs a no-op batch authorization check (always returns allowed)
func (c *NoopClient) BatchCheck(ctx context.Context, reqs ...*CheckRequest) (*CheckResponse, error) {
    c.logger.Debug("NoopClient: BatchCheck called", "count", len(reqs))
    return &CheckResponse{Allowed: true}, nil
}

// Write performs a no-op write
func (c *NoopClient) Write(ctx context.Context, req *WriteRequest) (*WriteResponse, error) {
    c.logger.Debug("NoopClient: Write called", "writes", len(req.Writes), "deletes", len(req.Deletes))
    return &WriteResponse{Success: true}, nil
}

// Read performs a no-op read
func (c *NoopClient) Read(ctx context.Context, req *ReadRequest) (*ReadResponse, error) {
    c.logger.Debug("NoopClient: Read called", "user", req.User, "relation", req.Relation, "object", req.Object)
    return &ReadResponse{Tuples: []Tuple{}}, nil
}

// Close is a no-op
func (c *NoopClient) Close() error {
    return nil
}*/
