package openfga

import (
    "context"

    openfga2 "github.com/openfga/go-sdk"
    "github.com/openfga/go-sdk/client"
)

// OpenFGAClientInterface mirrors the SdkClient interface from the OpenFGA Go SDK.
type OpenFGAClientInterface interface {
    /* Stores */

    // ListStores gets a paginated list of stores.
    ListStores(ctx context.Context) client.SdkClientListStoresRequestInterface
    // ListStoresExecute executes the ListStores request.
    ListStoresExecute(request client.SdkClientListStoresRequestInterface) (*client.ClientListStoresResponse, error)

    // CreateStore creates and initializes a store.
    CreateStore(ctx context.Context) client.SdkClientCreateStoreRequestInterface
    // CreateStoreExecute executes the CreateStore request.
    CreateStoreExecute(request client.SdkClientCreateStoreRequestInterface) (*client.ClientCreateStoreResponse, error)

    // GetStore gets information about the current store.
    GetStore(ctx context.Context) client.SdkClientGetStoreRequestInterface
    // GetStoreExecute executes the GetStore request.
    GetStoreExecute(request client.SdkClientGetStoreRequestInterface) (*client.ClientGetStoreResponse, error)

    // DeleteStore deletes a store.
    DeleteStore(ctx context.Context) client.SdkClientDeleteStoreRequestInterface
    // DeleteStoreExecute executes the DeleteStore request.
    DeleteStoreExecute(request client.SdkClientDeleteStoreRequestInterface) (*client.ClientDeleteStoreResponse, error)

    /* Authorization Models */

    // ReadAuthorizationModels reads all authorization models in the store.
    ReadAuthorizationModels(ctx context.Context) client.SdkClientReadAuthorizationModelsRequestInterface
    // ReadAuthorizationModelsExecute executes the ReadAuthorizationModels request.
    ReadAuthorizationModelsExecute(request client.SdkClientReadAuthorizationModelsRequestInterface) (*client.ClientReadAuthorizationModelsResponse, error)

    // WriteAuthorizationModel creates a new authorization model.
    WriteAuthorizationModel(ctx context.Context) client.SdkClientWriteAuthorizationModelRequestInterface
    // WriteAuthorizationModelExecute executes the WriteAuthorizationModel request.
    WriteAuthorizationModelExecute(request client.SdkClientWriteAuthorizationModelRequestInterface) (*client.ClientWriteAuthorizationModelResponse, error)

    // ReadAuthorizationModel reads a particular authorization model.
    ReadAuthorizationModel(ctx context.Context) client.SdkClientReadAuthorizationModelRequestInterface
    // ReadAuthorizationModelExecute executes the ReadAuthorizationModel request.
    ReadAuthorizationModelExecute(request client.SdkClientReadAuthorizationModelRequestInterface) (*client.ClientReadAuthorizationModelResponse, error)

    // ReadLatestAuthorizationModel reads the latest authorization model.
    ReadLatestAuthorizationModel(ctx context.Context) client.SdkClientReadLatestAuthorizationModelRequestInterface
    // ReadLatestAuthorizationModelExecute executes the ReadLatestAuthorizationModel request.
    ReadLatestAuthorizationModelExecute(request client.SdkClientReadLatestAuthorizationModelRequestInterface) (*client.ClientReadAuthorizationModelResponse, error)

    /* Relationship Tuples */

    // ReadChanges reads the list of historical relationship tuple writes and deletes.
    ReadChanges(ctx context.Context) client.SdkClientReadChangesRequestInterface
    // ReadChangesExecute executes the ReadChanges request.
    ReadChangesExecute(request client.SdkClientReadChangesRequestInterface) (*client.ClientReadChangesResponse, error)

    // Read reads relationship tuples stored in the database.
    Read(ctx context.Context) client.SdkClientReadRequestInterface
    // ReadExecute executes the Read request.
    ReadExecute(request client.SdkClientReadRequestInterface) (*client.ClientReadResponse, error)

    // Write creates and/or deletes relationship tuples to update the system state.
    Write(ctx context.Context) client.SdkClientWriteRequestInterface
    // WriteExecute executes the Write request.
    WriteExecute(request client.SdkClientWriteRequestInterface) (*client.ClientWriteResponse, error)

    // WriteTuples is a utility method around Write.
    WriteTuples(ctx context.Context) client.SdkClientWriteTuplesRequestInterface
    // WriteTuplesExecute executes the WriteTuples request.
    WriteTuplesExecute(request client.SdkClientWriteTuplesRequestInterface) (*client.ClientWriteResponse, error)

    // DeleteTuples is a utility method around Write.
    DeleteTuples(ctx context.Context) client.SdkClientDeleteTuplesRequestInterface
    // DeleteTuplesExecute executes the DeleteTuples request.
    DeleteTuplesExecute(request client.SdkClientDeleteTuplesRequestInterface) (*client.ClientWriteResponse, error)

    /* Relationship Queries */

    // Check checks if a user has a particular relation with an object.
    Check(ctx context.Context) client.SdkClientCheckRequestInterface
    // CheckExecute executes the Check request.
    CheckExecute(request client.SdkClientCheckRequestInterface) (*client.ClientCheckResponse, error)

    // ClientBatchCheck runs a set of checks (client-side).
    ClientBatchCheck(ctx context.Context) client.SdkClientBatchCheckClientRequestInterface
    // ClientBatchCheckExecute executes the client-side BatchCheck request.
    ClientBatchCheckExecute(request client.SdkClientBatchCheckClientRequestInterface) (*client.ClientBatchCheckClientResponse, error)

    // BatchCheck runs a set of checks on the server (server-side batch check).
    BatchCheck(ctx context.Context) client.SdkClientBatchCheckRequestInterface
    // BatchCheckExecute executes the server-side BatchCheck request.
    BatchCheckExecute(request client.SdkClientBatchCheckRequestInterface) (*openfga2.BatchCheckResponse, error)

    // Expand expands the relationships in userset tree format.
    Expand(ctx context.Context) client.SdkClientExpandRequestInterface
    // ExpandExecute executes the Expand request.
    ExpandExecute(request client.SdkClientExpandRequestInterface) (*client.ClientExpandResponse, error)

    // ListObjects lists the objects of a particular type a user has access to.
    ListObjects(ctx context.Context) client.SdkClientListObjectsRequestInterface
    // ListObjectsExecute executes the ListObjects request.
    ListObjectsExecute(request client.SdkClientListObjectsRequestInterface) (*client.ClientListObjectsResponse, error)

    // ListRelations lists the relations a user has on an object.
    ListRelations(ctx context.Context) client.SdkClientListRelationsRequestInterface
    // ListRelationsExecute executes the ListRelations request.
    ListRelationsExecute(request client.SdkClientListRelationsRequestInterface) (*client.ClientListRelationsResponse, error)

    // ListUsers lists all users of the given type that the object has a relation with.
    ListUsers(ctx context.Context) client.SdkClientListUsersRequestInterface
    // ListUsersExecute executes the ListUsers request.
    ListUsersExecute(r client.SdkClientListUsersRequestInterface) (*client.ClientListUsersResponse, error)

    // StreamedListObjects streams all objects of the given type that the user has a relation with.
    StreamedListObjects(ctx context.Context) client.SdkClientStreamedListObjectsRequestInterface
    // StreamedListObjectsExecute executes the StreamedListObjects request and returns a channel.
    StreamedListObjectsExecute(request client.SdkClientStreamedListObjectsRequestInterface) (*client.ClientStreamedListObjectsResponse, error)

    /* Assertions */

    // ReadAssertions reads assertions for a particular authorization model.
    ReadAssertions(ctx context.Context) client.SdkClientReadAssertionsRequestInterface
    // ReadAssertionsExecute executes the ReadAssertions request.
    ReadAssertionsExecute(request client.SdkClientReadAssertionsRequestInterface) (*client.ClientReadAssertionsResponse, error)

    // WriteAssertions updates the assertions for a particular authorization model.
    WriteAssertions(ctx context.Context) client.SdkClientWriteAssertionsRequestInterface
    // WriteAssertionsExecute executes the WriteAssertions request.
    WriteAssertionsExecute(request client.SdkClientWriteAssertionsRequestInterface) (*client.ClientWriteAssertionsResponse, error)

    /* Store/Model ID management */

    // SetAuthorizationModelId sets the Authorization Model ID.
    SetAuthorizationModelId(authorizationModelId string) error
    // GetAuthorizationModelId retrieves the Authorization Model ID.
    GetAuthorizationModelId() (string, error)
    // SetStoreId sets the Store ID.
    SetStoreId(storeId string) error
    // GetStoreId retrieves the Store ID.
    GetStoreId() (string, error)
}
