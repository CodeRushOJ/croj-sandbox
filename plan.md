```
croj-sandbox/
├── cmd/
│   └── croj-sandbox/      # Main application entry point
│       └── main.go
├── internal/              # Internal packages (core logic)
│   ├── sandbox/
│   │   ├── runner.go         # Orchestrator
│   │   ├── compiler.go       # Local Go compiler logic
│   │   ├── executor.go       # Local executor logic (using os/exec)
│   │   ├── result.go         # Result struct and statuses
│   │   └── config.go         # Configuration
│   └── util/               # Utility functions
│       └── tempdir.go       # Temp directory management helper
├── go.mod
├── go.sum
└── README.md              # (Optional but recommended)
```
v0.2 - Introduce Basic Containerization (Execution Only)

Goal: Replace local execution (os/exec) with Docker container execution for the user's compiled binary. Keep local compilation for now.

Steps:

Create Dockerfile.exec (minimal Alpine base, non-root user, workdir) - Similar to previous container plans.

Add Docker SDK dependency (github.com/docker/docker).

Update sandbox.Config: Add ExecImageName, container resource limits (CPUQuota, MemoryLimitMB, PidsLimit), ContainerTimeout.

Modify sandbox.Executor:

Initialize Docker client (passed from Runner).

Execute function:

Create and start a container using ExecImageName and resource limits.

Use archive.go (add this file back) and CopyToContainer to copy the hostBinaryPath into the container.

Use ContainerExecCreate/Attach/Inspect to run the binary inside the container (similar logic to the first container plan's executeInContainer).

Implement actual MemoryLimitExceeded detection by inspecting container state (OOMKilled).

Use ContainerStats to get a real MemoryUsedKB value.

Handle Docker API errors.

Update sandbox.Runner:

Initialize and pass Docker client to Executor.

Implement Close() to close the Docker client.

Update main.go: Adjust config, build the Dockerfile.exec image (docker build -t <ExecImageName> -f Dockerfile.exec .).

v0.3 - File I/O within Container

Goal: Allow user code running in the container to read input files and write output files.

Steps:

Config: Add ContainerInputDir, ContainerOutputDir, MaxTotalOutputSizeKB (optional, for combined stdout/stderr/files).

Runner.Run Interface: Accept input files (e.g., map[string]string for filename -> content).

Runner: Write input files to hostRunDir/input.

Executor:

Tar the hostRunDir/input directory.

CopyToContainer the input tarball to Config.ContainerInputDir.

After successful execution (Accepted, maybe specific RuntimeError), use CopyFromContainer(ContainerOutputDir) to retrieve output files as a tar stream.

Untar the output stream into hostRunDir/output.

Result: Add OutputFiles map[string]string field.

Runner: Read files from hostRunDir/output and populate Result.OutputFiles. Handle potential errors during copy/untar.

v0.4 - Enhanced Security & Isolation (Container)

Goal: Apply security best practices to the execution container.

Steps: (Requires testing, especially Seccomp on Mac)

Seccomp: Define a profile, apply via HostConfig.SecurityOpt.

Capabilities: Drop all capabilities (CapDrop: ["ALL"]).

Read-Only RootFS: Set ReadonlyRootfs: true, mount necessary tmpfs (/tmp, /app with size limits) via HostConfig.Tmpfs. Ensure /app (or where binary/files are) is writable within the tmpfs mount.

Network: Ensure network is disabled (NetworkMode: "none") unless explicitly needed and configured.

v0.5 - Multi-language Support (Initial)

Goal: Abstract language-specific steps, add Python as a second language.

Steps:

Refactor Compiler and Executor logic into language-specific components or strategies. Maybe an interface LanguageHandler with Compile() and GetExecConfig() methods.

Python:

Compile: No-op or basic syntax check (python -m py_compile). Source file (.py) is the "binary" to copy.

GetExecConfig: Returns command ["python", "main.py"] and potentially required execution image name.

Update Dockerfile.exec (or create Dockerfile.exec.py) to include the Python interpreter.

Runner: Accept a language parameter, select the appropriate handler.

Config: Allow language-specific image overrides.

v0.6+ - API, Production Readiness, etc. (Similar to previous plan: HTTP API, Logging, Metrics, Caching, Container Pooling, Packaging the Runner itself).

