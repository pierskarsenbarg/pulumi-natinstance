# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Pulumi native provider (built with `pulumi-go-provider`) called `fcknat`, exposing a `NatInstance` component resource that stands up a [fck-nat](https://github.com/AndrewGuenther/fck-nat) instance (a cheaper alternative to AWS NAT Gateway) using EC2, an Auto Scaling Group, and IAM resources. Multi-language SDKs (Go, Node.js, Python, .NET) are generated from the provider schema into `sdk/`.

## Commands (via Task, see `taskfile.yaml`)

- `task build_provider` — `go mod tidy` + build the provider binary to `bin/pulumi-resource-fcknat` (requires `pulumictl` for versioning)
- `task get_schema` — builds the provider then extracts `schema.json` via `pulumi package get-schema`
- `task go_sdk` / `task nodejs_sdk` / `task python_sdk` / `task dotnet_sdk` — regenerate a single language SDK from the built provider (each wipes and regenerates its `sdk/<lang>` directory)
- `task build_sdks` — builds the provider, then regenerates all four SDKs
- `task ensure` — `go mod tidy` in both the root module and `sdk/`
- `task watch` — rebuilds the provider on any `.go` file change (500ms debounce)
- `task clean` — removes `./bin`

There are two independent Go modules: the root (`github.com/pierskarsenbarg/pulumi-fcknat`) and `sdk/` (`github.com/pierskarsenbarg/pulumi-fcknat/sdk`). Run `go build ./...` / `go vet ./...` from the relevant module root; there is no test suite in this repo currently.

CI (`.github/workflows/main.yaml`, `pr.yaml`) just runs `task build_sdks` on push to main and on PR open/reopen — it does not run tests or lint.

## Architecture

- `main.go` wires up the provider via `infer.Provider`, registering `pkg.NatInstance` as the sole component resource. It also carries per-language codegen metadata (Go import path, Node/Python/C# dependency versions) that must be updated here when target SDK dependency versions change — this is the single source of truth for generated-SDK package metadata, not the individual `sdk/<lang>` directories (those are regenerated wholesale by the `task *_sdk` targets).
- `ModuleMap` remaps the `pkg` folder to the schema module `index` — required because Pulumi component/resource token modules are derived from Go package/folder names, and the folder here is named `pkg`.
- `pkg/fcknat.go` contains the actual `NatInstance.Construct` logic: it resolves the target VPC (default VPC if `VpcId` unset), determines whether the VPC has non-default routing (multiple route tables) to decide whether to source subnet IDs from route tables (picking the one with an IGW route, i.e. public subnets) or from all VPC subnets, then creates: a security group (allow-all in from VPC CIDR, allow-all out), a network interface with `SourceDestCheck` disabled, an IAM role/instance profile allowing `ec2:AttachNetworkInterface`/`ModifyNetworkInterfaceAttribute`/`AssociateAddress`/`DisassociateAddress`, a launch template using the latest `fck-nat-al2023-*` AMI (owned by `568608671756`) whose user-data wires the ENI ID into `/etc/fck-nat.conf`, and a single-instance Auto Scaling Group pinned to the first (sorted) subnet ID.
- `pkg/account.go` and `pkg/config.go` are leftover boilerplate from the `pulumi-go-provider` scaffold (an `Account` custom resource, `GetAccount` function, and provider `Config`) — not part of the `fcknat` provider's registered surface in `main.go`, and `examples/simple/Pulumi.yaml` still references this scaffold (`base:Account`, `base:getAccount`) rather than `NatInstance`. Treat these as unwired scaffolding, not documentation of current behavior, when reasoning about what the provider actually does.
- Generated SDKs in `sdk/{go,nodejs,python,dotnet}` are build artifacts of `pulumi package gen-sdk` — don't hand-edit them; change `main.go`/`pkg/*.go` and regenerate instead.
