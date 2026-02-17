package docker

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Compile-time interface satisfaction checks
// ---------------------------------------------------------------------------
//
// These declarations verify at compile time that both the production type
// (DockerClientManager) and the test mock (MockDockerManager) satisfy each
// of the new split interfaces as well as the redefined composite interface.
//
// If any method is missing from a concrete type or mismatches the interface
// signature, the build will fail before any test runs.
// ---------------------------------------------------------------------------

var _ ContainerManager = (*DockerClientManager)(nil)
var _ NetworkManager = (*DockerClientManager)(nil)
var _ DockerManager = (*DockerClientManager)(nil)

var _ ContainerManager = (*MockDockerManager)(nil)
var _ NetworkManager = (*MockDockerManager)(nil)
var _ DockerManager = (*MockDockerManager)(nil)

// ---------------------------------------------------------------------------
// Structural equivalence via reflection
// ---------------------------------------------------------------------------

// Test_DockerManager_MethodCount verifies that the composite DockerManager
// interface exposes exactly the sum of ContainerManager and NetworkManager
// methods with no extras and no missing methods.
func Test_DockerManager_MethodCount(t *testing.T) {
	dockerManagerType := reflect.TypeOf((*DockerManager)(nil)).Elem()
	containerManagerType := reflect.TypeOf((*ContainerManager)(nil)).Elem()
	networkManagerType := reflect.TypeOf((*NetworkManager)(nil)).Elem()

	got := dockerManagerType.NumMethod()
	want := containerManagerType.NumMethod() + networkManagerType.NumMethod()

	if got != want {
		t.Errorf(
			"DockerManager.NumMethod() = %d, want ContainerManager.NumMethod() (%d) + NetworkManager.NumMethod() (%d) = %d",
			got, containerManagerType.NumMethod(), networkManagerType.NumMethod(), want,
		)
	}
}

// Test_ContainerManager_NetworkManager_NoOverlap verifies that the
// ContainerManager and NetworkManager interfaces share zero method names.
// Any overlap would indicate an incorrect interface split.
func Test_ContainerManager_NetworkManager_NoOverlap(t *testing.T) {
	containerManagerType := reflect.TypeOf((*ContainerManager)(nil)).Elem()
	networkManagerType := reflect.TypeOf((*NetworkManager)(nil)).Elem()

	containerMethods := make(map[string]struct{}, containerManagerType.NumMethod())
	for i := 0; i < containerManagerType.NumMethod(); i++ {
		containerMethods[containerManagerType.Method(i).Name] = struct{}{}
	}

	var overlapping []string
	for i := 0; i < networkManagerType.NumMethod(); i++ {
		name := networkManagerType.Method(i).Name
		if _, exists := containerMethods[name]; exists {
			overlapping = append(overlapping, name)
		}
	}

	if len(overlapping) > 0 {
		t.Errorf(
			"ContainerManager and NetworkManager share %d overlapping method(s): %v",
			len(overlapping), overlapping,
		)
	}
}

// Test_ContainerManager_MethodCount verifies that ContainerManager defines
// exactly 10 methods corresponding to the container operations.
func Test_ContainerManager_MethodCount(t *testing.T) {
	containerManagerType := reflect.TypeOf((*ContainerManager)(nil)).Elem()

	got := containerManagerType.NumMethod()
	want := 10

	if got != want {
		t.Errorf("ContainerManager.NumMethod() = %d, want %d", got, want)

		// List the actual methods for diagnostic clarity.
		t.Log("ContainerManager methods:")
		for i := 0; i < containerManagerType.NumMethod(); i++ {
			t.Logf("  - %s", containerManagerType.Method(i).Name)
		}
	}
}

// Test_NetworkManager_MethodCount verifies that NetworkManager defines
// exactly 6 methods corresponding to the network operations.
func Test_NetworkManager_MethodCount(t *testing.T) {
	networkManagerType := reflect.TypeOf((*NetworkManager)(nil)).Elem()

	got := networkManagerType.NumMethod()
	want := 6

	if got != want {
		t.Errorf("NetworkManager.NumMethod() = %d, want %d", got, want)

		// List the actual methods for diagnostic clarity.
		t.Log("NetworkManager methods:")
		for i := 0; i < networkManagerType.NumMethod(); i++ {
			t.Logf("  - %s", networkManagerType.Method(i).Name)
		}
	}
}

// ---------------------------------------------------------------------------
// Method name verification
// ---------------------------------------------------------------------------

// Test_ContainerManager_ExpectedMethods verifies that ContainerManager
// contains exactly the 10 expected method names from the specification.
func Test_ContainerManager_ExpectedMethods(t *testing.T) {
	containerManagerType := reflect.TypeOf((*ContainerManager)(nil)).Elem()

	expectedMethods := []string{
		"ListContainers",
		"InspectContainer",
		"StartContainer",
		"StopContainer",
		"RestartContainer",
		"RemoveContainer",
		"CreateContainer",
		"PullImage",
		"GetLogs",
		"GetStats",
	}

	actualMethods := make(map[string]struct{}, containerManagerType.NumMethod())
	for i := 0; i < containerManagerType.NumMethod(); i++ {
		actualMethods[containerManagerType.Method(i).Name] = struct{}{}
	}

	for _, name := range expectedMethods {
		if _, ok := actualMethods[name]; !ok {
			t.Errorf("ContainerManager is missing expected method %q", name)
		}
	}

	// Check for unexpected methods.
	expectedSet := make(map[string]struct{}, len(expectedMethods))
	for _, name := range expectedMethods {
		expectedSet[name] = struct{}{}
	}
	for name := range actualMethods {
		if _, ok := expectedSet[name]; !ok {
			t.Errorf("ContainerManager has unexpected method %q", name)
		}
	}
}

// Test_NetworkManager_ExpectedMethods verifies that NetworkManager
// contains exactly the 6 expected method names from the specification.
func Test_NetworkManager_ExpectedMethods(t *testing.T) {
	networkManagerType := reflect.TypeOf((*NetworkManager)(nil)).Elem()

	expectedMethods := []string{
		"ListNetworks",
		"InspectNetwork",
		"CreateNetwork",
		"RemoveNetwork",
		"ConnectNetwork",
		"DisconnectNetwork",
	}

	actualMethods := make(map[string]struct{}, networkManagerType.NumMethod())
	for i := 0; i < networkManagerType.NumMethod(); i++ {
		actualMethods[networkManagerType.Method(i).Name] = struct{}{}
	}

	for _, name := range expectedMethods {
		if _, ok := actualMethods[name]; !ok {
			t.Errorf("NetworkManager is missing expected method %q", name)
		}
	}

	// Check for unexpected methods.
	expectedSet := make(map[string]struct{}, len(expectedMethods))
	for _, name := range expectedMethods {
		expectedSet[name] = struct{}{}
	}
	for name := range actualMethods {
		if _, ok := expectedSet[name]; !ok {
			t.Errorf("NetworkManager has unexpected method %q", name)
		}
	}
}

// ---------------------------------------------------------------------------
// DockerManager embeds both sub-interfaces
// ---------------------------------------------------------------------------

// Test_DockerManager_ContainsAllSubInterfaceMethods verifies that every method
// from ContainerManager and NetworkManager is present in DockerManager, which
// confirms the embedding relationship is correct.
func Test_DockerManager_ContainsAllSubInterfaceMethods(t *testing.T) {
	dockerManagerType := reflect.TypeOf((*DockerManager)(nil)).Elem()
	containerManagerType := reflect.TypeOf((*ContainerManager)(nil)).Elem()
	networkManagerType := reflect.TypeOf((*NetworkManager)(nil)).Elem()

	dockerMethods := make(map[string]struct{}, dockerManagerType.NumMethod())
	for i := 0; i < dockerManagerType.NumMethod(); i++ {
		dockerMethods[dockerManagerType.Method(i).Name] = struct{}{}
	}

	// Every ContainerManager method must be in DockerManager.
	for i := 0; i < containerManagerType.NumMethod(); i++ {
		name := containerManagerType.Method(i).Name
		if _, ok := dockerMethods[name]; !ok {
			t.Errorf("DockerManager is missing ContainerManager method %q", name)
		}
	}

	// Every NetworkManager method must be in DockerManager.
	for i := 0; i < networkManagerType.NumMethod(); i++ {
		name := networkManagerType.Method(i).Name
		if _, ok := dockerMethods[name]; !ok {
			t.Errorf("DockerManager is missing NetworkManager method %q", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Interface assignability checks via reflection
// ---------------------------------------------------------------------------

// Test_DockerManager_ImplementsContainerManager verifies that any value
// satisfying DockerManager can be assigned to ContainerManager.
func Test_DockerManager_ImplementsContainerManager(t *testing.T) {
	dockerManagerType := reflect.TypeOf((*DockerManager)(nil)).Elem()
	containerManagerType := reflect.TypeOf((*ContainerManager)(nil)).Elem()

	if !dockerManagerType.Implements(containerManagerType) {
		t.Error("DockerManager does not implement ContainerManager")
	}
}

// Test_DockerManager_ImplementsNetworkManager verifies that any value
// satisfying DockerManager can be assigned to NetworkManager.
func Test_DockerManager_ImplementsNetworkManager(t *testing.T) {
	dockerManagerType := reflect.TypeOf((*DockerManager)(nil)).Elem()
	networkManagerType := reflect.TypeOf((*NetworkManager)(nil)).Elem()

	if !dockerManagerType.Implements(networkManagerType) {
		t.Error("DockerManager does not implement NetworkManager")
	}
}
