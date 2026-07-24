package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"github.com/CodeRushOJ/croj-sandbox/internal/security"
	"golang.org/x/sys/unix"
)

const (
	gateFD   = 3
	statusFD = 4
)

var environmentName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

type environmentFlags []string

func (values *environmentFlags) String() string {
	return strings.Join(*values, ",")
}

func (values *environmentFlags) Set(value string) error {
	name, _, ok := strings.Cut(value, "=")
	if !ok || !environmentName.MatchString(name) {
		return fmt.Errorf("invalid target environment entry")
	}
	switch name {
	case "LD_PRELOAD", "LD_LIBRARY_PATH", "GCONV_PATH":
		return fmt.Errorf("unsafe target environment entry %s", name)
	}
	*values = append(*values, value)
	return nil
}

func main() {
	if err := run(); err != nil {
		// Never echo contestant argv, environment values, source, or paths.
		_, _ = fmt.Fprintln(os.Stderr, "sandbox launcher: isolation setup failed")
		os.Exit(125)
	}
}

func run() error {
	flags := flag.NewFlagSet("sandbox-exec", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	language := flags.String("language", "", "language security profile")
	workDir := flags.String("work-dir", "", "request-private working directory")
	var targetEnvironment environmentFlags
	flags.Var(&targetEnvironment, "target-env", "target KEY=VALUE")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	target := flags.Args()
	if *language == "" || *workDir == "" || len(target) == 0 {
		return errors.New("language, work-dir, and target command are required")
	}

	gate := os.NewFile(gateFD, "isolation-gate")
	status := os.NewFile(statusFD, "isolation-status")
	if gate == nil || status == nil {
		return errors.New("isolation handshake descriptors are required")
	}
	defer gate.Close()
	defer status.Close()

	var signal [1]byte
	if count, err := gate.Read(signal[:]); err != nil || count != 1 || signal[0] != 1 {
		return errors.New("supervisor did not authorize execution")
	}

	targetPath, err := exec.LookPath(target[0])
	if err != nil {
		return errors.New("target executable is unavailable")
	}
	if err := os.Chdir(*workDir); err != nil {
		return errors.New("enter request working directory")
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return errors.New("set no-new-privileges")
	}

	uid := security.IsolatedUIDForProcess(os.Getpid())
	if err := syscall.Setgroups([]int{}); err != nil {
		return errors.New("drop supplementary groups")
	}
	if err := syscall.Setgid(uid); err != nil {
		return errors.New("drop group identity")
	}
	if err := syscall.Setuid(uid); err != nil {
		return errors.New("drop user identity")
	}
	syscall.Umask(0o077)

	profile := security.ProfileForLanguage(*language)
	profile.DisableNetwork = true
	profile.NoNewPrivileges = true
	if err := security.ApplySeccompFilters(profile); err != nil {
		return errors.New("apply seccomp policy")
	}
	if _, err := status.Write([]byte{1}); err != nil {
		return errors.New("report isolation readiness")
	}
	if err := status.Close(); err != nil {
		return errors.New("close isolation status")
	}

	environment := []string{
		"PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"HOME=" + *workDir,
		"TMPDIR=" + *workDir,
		"GOCACHE=" + *workDir + "/.go-build-cache",
	}
	environment = append(environment, targetEnvironment...)
	return unix.Exec(targetPath, target, environment)
}
