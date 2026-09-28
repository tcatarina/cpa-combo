// Command cpa-plugin-combos is a CLIProxyAPI plugin that exposes named model
// fallbacks ("combos") as ordinary models.
package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	pluginName     = "combos"
	executorID     = "combos"
	storeFileName  = "combos.json"
	comboNamespace = "combo"
)

// pluginVersion is stamped by the release workflow from the tag. A build that
// was not stamped reports "dev" rather than a version that can silently go stale.
var pluginVersion = "dev"

var (
	hostPtr     atomic.Pointer[C.cliproxy_host_api]
	traceOn     = os.Getenv("COMBOS_TRACE") != ""
	runtimeLock sync.RWMutex
	runtime     = newRuntimeState()
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	ModelRouter           bool     `json:"model_router"`
	ModelRegistrar        bool     `json:"model_registrar"`
	Executor              bool     `json:"executor"`
	ExecutorModelScope    string   `json:"executor_model_scope"`
	ExecutorInputFormats  []string `json:"executor_input_formats"`
	ExecutorOutputFormats []string `json:"executor_output_formats"`
	ManagementAPI         bool     `json:"management_api"`
}

type rpcModelRouteRequest struct {
	pluginapi.ModelRouteRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type rpcExecutorRequest struct {
	pluginapi.ExecutorRequest
	StreamID       string `json:"stream_id,omitempty"`
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type executorStreamResponse struct {
	Headers http.Header                     `json:"headers,omitempty"`
	Chunks  []pluginapi.ExecutorStreamChunk `json:"chunks,omitempty"`
}

type rpcManagementRequest struct {
	pluginapi.ManagementRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	hostPtr.Store(host)
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	buf := C.CBytes(raw)
	response.ptr = buf
	response.len = C.size_t(len(raw))
}

func handleMethod(method string, request []byte) ([]byte, error) {
	if traceOn && method != pluginabi.MethodPluginRegister && method != pluginabi.MethodPluginReconfigure {
		_, _ = callHost(pluginabi.MethodHostLog, map[string]any{
			"level":   "debug",
			"message": "combos: dispatch " + method + " payload=" + itoa(len(request)),
		})
	}
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		if err := runtime.configure(request); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodModelRoute:
		return handleModelRoute(request)
	case pluginabi.MethodModelRegister:
		return handleModelRegister(request)
	case pluginabi.MethodExecutorIdentifier:
		return okEnvelope(map[string]string{"identifier": executorID})
	case pluginabi.MethodExecutorExecute:
		return handleExecute(request, false)
	case pluginabi.MethodExecutorExecuteStream:
		return handleExecute(request, true)
	case pluginabi.MethodExecutorCountTokens:
		return okEnvelope(pluginapi.ExecutorResponse{Payload: []byte(`{"input_tokens":0}`)})
	case pluginabi.MethodExecutorHTTPRequest:
		return errorEnvelope("unsupported", "combos does not serve raw HTTP"), nil
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRouteSetResponse())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	case pluginabi.MethodUsageHandle:
		return okEnvelope(map[string]any{})
	default:
		return errorEnvelope("unsupported_method", "method not implemented: "+method), nil
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "tcatarina",
			GitHubRepository: "https://github.com/tcatarina/cpa-combo",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "enabled", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Enable combo routing."},
			},
		},
		Capabilities: registrationCapability{
			ModelRouter:           true,
			ModelRegistrar:        true,
			Executor:              true,
			ExecutorModelScope:    string(pluginapi.ExecutorModelScopeStatic),
			ExecutorInputFormats:  []string{"openai", "claude", "gemini", "responses"},
			ExecutorOutputFormats: []string{"openai", "claude", "gemini", "responses"},
			ManagementAPI:         true,
		},
	}
}

func callHost(method string, payload any) (json.RawMessage, error) {
	host := hostPtr.Load()
	if host == nil {
		return nil, errHostUnavailable
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))

	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		buf := C.CBytes(rawPayload)
		if buf == nil {
			return nil, errHostCall
		}
		defer C.free(buf)
		requestPtr = (*C.uint8_t)(buf)
	}

	var response C.cliproxy_buffer
	response.ptr = nil
	response.len = 0

	code := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)

	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}

	if code != 0 {
		return nil, &hostError{msg: "host callback " + method + " returned code " + itoa(int(code))}
	}
	if len(rawResponse) == 0 {
		return nil, &hostError{msg: "host callback " + method + " returned no response"}
	}
	return rawResponse, nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type hostError struct {
	msg    string
	status int
	code   string
}

func (e *hostError) Error() string { return e.msg }

var (
	errHostUnavailable = &hostError{msg: "host api unavailable"}
	errHostCall        = &hostError{msg: "host callback failed"}
)

type hostEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *envelopeError  `json:"error"`
}

func callHostResult(method string, payload any, out any) error {
	raw, err := callHost(method, payload)
	if err != nil {
		return err
	}
	var env hostEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return &hostError{msg: "decode " + method + " envelope: " + err.Error()}
	}
	if !env.OK {
		msg := method + " failed"
		status := 0
		if env.Error != nil {
			msg = env.Error.Message
			status = env.Error.HTTPStatus
		}
		return &hostError{msg: msg, status: status, code: method}
	}
	if out == nil {
		return nil
	}
	if len(env.Result) == 0 {
		return &hostError{msg: method + " returned an empty result"}
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return &hostError{msg: "decode " + method + " result: " + err.Error()}
	}
	return nil
}

func okEnvelope(result any) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return errorEnvelope("marshal_failed", err.Error()), nil
	}
	out, err := json.Marshal(envelope{OK: true, Result: raw})
	if err != nil {
		return errorEnvelope("marshal_failed", err.Error()), nil
	}
	return out, nil
}

func errorEnvelope(code, message string) []byte {
	out, err := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"internal_error","message":"envelope marshal failed"}}`)
	}
	return out
}
