# ExtMCP protocol binding

These protobuf definitions and generated Go bindings come from agentgateway
v1.7.0, commit `26f5aebe99a36a037fe54f8080a608aca5547e15`.
Only the generated Go package name is changed to `wire`.
The original Apache 2.0 license is retained alongside them.

Source: https://github.com/agentgateway/agentgateway/tree/v1.7.0/api

Keep this binding aligned with the pinned gateway image. It does not include
the gateway implementation, controller or unrelated API protobuf dependencies.
