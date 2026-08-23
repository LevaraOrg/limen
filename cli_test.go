package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binPath is the compiled limen, built once in TestMain. Everything below runs
// the real binary rather than calling functions: the contract this tool has with
// a .zshrc is about exit codes and stdout, and those are only real in a process.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "limen-build-*")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "limen")
	// With LIMEN_COVDIR set (make cover), the binary is built instrumented so
	// the CLI tests count towards coverage — they are where most behaviour
	// lives, and a metric that cannot see them reports the wrong number. Its
	// own variable rather than GOCOVERDIR, because `go test` overrides that
	// one for the test process and the children would write into a temp dir
	// that is harvested and discarded.
	buildArgs := []string{"build", "-o", binPath, "."}
	if os.Getenv("LIMEN_COVDIR") != "" {
		buildArgs = []string{"build", "-cover", "-o", binPath, "."}
	}
	build := exec.Command("go", buildArgs...)
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("go build failed: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout string
	stderr string
	code   int
}

func runLimen(t *testing.T, dir string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	// A deliberately minimal environment: HOME is needed for tilde expansion,
	// PATH for security(1). ANTHROPIC_API_KEY stays unset unless a test sets it.
	// XDG_STATE_HOME points into the test directory so `shell` and `register`
	// never touch the real registry under ~/.local/state; a test that needs a
	// shared registry across calls passes its own value in env instead.
	base := []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}
	// Rides along only under `make cover`, so the instrumented binary has a
	// place to drop its counters; unset otherwise, and nothing changes.
	if d := os.Getenv("LIMEN_COVDIR"); d != "" {
		base = append(base, "GOCOVERDIR="+d)
	}
	stateSet := false
	for _, e := range env {
		if strings.HasPrefix(e, "XDG_STATE_HOME=") {
			stateSet = true
		}
	}
	if !stateSet {
		base = append(base, "XDG_STATE_HOME="+filepath.Join(dir, ".limen-test-state"))
	}
	cmd.Env = append(base, env...)

	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running limen: %v", err)
	}
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// tempDir returns a temp directory with symlinks resolved. On macOS t.TempDir()
// hands back /var/... while the child process's getwd reports /private/var/...,
// so comparing paths literally would fail on the symlink rather than on limen.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// project lays out a .limen/ tree and returns the nested working directory.
func project(t *testing.T, body string) (root, nested string) {
	t.Helper()
	root = tempDir(t)
	write(t, filepath.Join(root, ".limen", "limen.yaml"), body)
	nested = filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, nested
}

const fullConfig = `label: tessera
purpose: Product strategy — role design and presentations
topics: design-thinking, customer-journey
actor: Matthias Wegner
githubUser: leo81
claudeConfigDir: ~/.claude-work
gcloudAccount: leo@example.com
gcloudProject: my-project-123
provider: anthropic
model: claude-opus-5
gateway: http://localhost:8787
keychainService: limen-anthropic
`

func TestCLIShowFromNestedDirectory(t *testing.T) {
	root, nested := project(t, fullConfig)
	r := runLimen(t, nested, nil, "show")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	for _, want := range []string{root, "tessera", "leo81", "my-project-123", ".limen/limen.yaml"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("show output missing %q:\n%s", want, r.stdout)
		}
	}
}

func TestCLIFlatLegacyLayoutStillLoads(t *testing.T) {
	// The pre-0.4 flat layout keeps working until `limen migrate` lifts it;
	// its notes stay next to it in the root so both move together later.
	root := tempDir(t)
	write(t, filepath.Join(root, ".limen.yaml"), fullConfig)

	r := runLimen(t, root, nil, "show")
	if r.code != 0 || !strings.Contains(r.stdout, "limen migrate") {
		t.Errorf("show should load the flat layout and point at migrate:\n%s", r.stdout)
	}
	if r := runLimen(t, root, nil, "note", "alte Welt"); r.code != 0 {
		t.Fatalf("note exit %d: %s", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "LIMEN.md")); err != nil {
		t.Error("flat layout must keep its notes in the root LIMEN.md")
	}
}

func TestCLIJSONIsValidAndComplete(t *testing.T) {
	root, nested := project(t, fullConfig)
	r := runLimen(t, nested, nil, "json")
	if r.code != 0 {
		t.Fatalf("exit %d", r.code)
	}
	if !strings.Contains(r.stdout, `"root":"`+root+`"`) {
		t.Errorf("json root wrong:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, `"gateway":"http://localhost:8787"`) {
		t.Errorf("json gateway wrong:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, `"api_key_in_config":false`) {
		t.Errorf("json should report no plaintext key:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, `"purpose":"Product strategy — role design and presentations"`) {
		t.Errorf("json purpose wrong:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, `"topics":["design-thinking","customer-journey"]`) {
		t.Errorf("json topics wrong:\n%s", r.stdout)
	}
}

func TestCLIShellIsEvaluable(t *testing.T) {
	_, nested := project(t, fullConfig)
	r := runLimen(t, nested, nil, "shell")
	if r.code != 0 {
		t.Fatalf("exit %d", r.code)
	}

	// The real contract: the output must survive `eval` in a shell.
	script := r.stdout + "\nprintf '%s|%s|%s' \"$LIMEN_LABEL\" \"$CLOUDSDK_CORE_PROJECT\" \"$LIMEN_SEGMENT\"\n"
	out, err := exec.Command("/bin/sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("eval failed: %v\nscript:\n%s", err, script)
	}
	want := "tessera|my-project-123|tessera · leo81 · claude-opus-5"
	if string(out) != want {
		t.Errorf("after eval got %q, want %q", out, want)
	}
}

func TestCLIShellSurvivesAQuoteInAValue(t *testing.T) {
	_, nested := project(t, "label: it's fine\nprovider: anthropic\n")
	r := runLimen(t, nested, nil, "shell")
	script := r.stdout + "\nprintf '%s' \"$LIMEN_LABEL\"\n"
	out, err := exec.Command("/bin/sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("eval failed: %v\nscript:\n%s", err, script)
	}
	if string(out) != "it's fine" {
		t.Errorf("got %q, want %q", out, "it's fine")
	}
}

func TestCLIPromptDoesNotTouchTheKeychain(t *testing.T) {
	_, nested := project(t, fullConfig)
	// A PATH without security(1) proves prompt does not need it.
	r := runLimen(t, nested, []string{"PATH=/nonexistent"}, "prompt")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	if strings.TrimSpace(r.stdout) != "tessera · leo81 · claude-opus-5" {
		t.Errorf("prompt = %q", r.stdout)
	}
}

func TestCLIWithoutContextIsSafeToCallFromAStartupFile(t *testing.T) {
	dir := tempDir(t)

	if r := runLimen(t, dir, nil, "json"); r.code != 0 || strings.TrimSpace(r.stdout) != "{}" {
		t.Errorf("json without context: exit %d out %q, want 0 and {}", r.code, r.stdout)
	}
	if r := runLimen(t, dir, nil, "shell"); r.code != 0 || r.stdout != "" {
		t.Errorf("shell without context: exit %d out %q, want 0 and empty", r.code, r.stdout)
	}
	if r := runLimen(t, dir, nil, "prompt"); r.code != 0 || strings.TrimSpace(r.stdout) != "" {
		t.Errorf("prompt without context: exit %d out %q", r.code, r.stdout)
	}
	if r := runLimen(t, dir, nil, "root"); r.code != 0 || strings.TrimSpace(r.stdout) != "" {
		t.Errorf("root without context: exit %d out %q", r.code, r.stdout)
	}
	// show is the one that must complain, because it was asked a direct question.
	if r := runLimen(t, dir, nil, "show"); r.code == 0 {
		t.Error("show without context should exit non-zero")
	}
}

func TestCLILegacyOrcaTree(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, ".orca", "config.yaml"),
		"---\nprovider: anthropic\nmodel: claude-opus-4-5\n")
	write(t, filepath.Join(root, ".orca", "identity.yaml"),
		"---\nactorId: \"abc\"\nname: \"Leo\"\n")

	r := runLimen(t, root, nil, "show")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %s", r.code, r.stderr)
	}
	for _, want := range []string{"Leo", "claude-opus-4-5", "legacy"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestCLINeverPrintsAPlaintextKey(t *testing.T) {
	_, nested := project(t, "label: leaky\nprovider: anthropic\napiKey: sk-ant-SECRETVALUE\n")

	for _, cmd := range []string{"show", "json", "prompt"} {
		r := runLimen(t, nested, nil, cmd)
		if strings.Contains(r.stdout, "SECRETVALUE") || strings.Contains(r.stderr, "SECRETVALUE") {
			t.Fatalf("%s leaked the key:\n%s%s", cmd, r.stdout, r.stderr)
		}
	}
	if r := runLimen(t, nested, nil, "json"); !strings.Contains(r.stdout, `"api_key_in_config":true`) {
		t.Errorf("json should flag the leak:\n%s", r.stdout)
	}
	if r := runLimen(t, nested, nil, "prompt"); !strings.Contains(r.stdout, "!key-in-config") {
		t.Errorf("prompt should mark the leak:\n%s", r.stdout)
	}
	// shell may export it — that is its job — but only from env or keychain,
	// never from the file.
	if r := runLimen(t, nested, nil, "shell"); strings.Contains(r.stdout, "SECRETVALUE") {
		t.Error("shell exported the key straight from the config file")
	}
}

func TestCLIShellExportsTheKeyFromTheEnvironment(t *testing.T) {
	_, nested := project(t, fullConfig)
	r := runLimen(t, nested, []string{"ANTHROPIC_API_KEY=sk-from-env"}, "shell")
	if !strings.Contains(r.stdout, "export ANTHROPIC_API_KEY='sk-from-env'") {
		t.Errorf("expected the env key to be re-exported:\n%s", r.stdout)
	}
}

func TestCLIRootPrintsTheProjectRoot(t *testing.T) {
	root, nested := project(t, fullConfig)
	r := runLimen(t, nested, nil, "root")
	if strings.TrimSpace(r.stdout) != root {
		t.Errorf("root = %q, want %q", strings.TrimSpace(r.stdout), root)
	}
}

func TestCLIInitWritesAndRefusesToOverwrite(t *testing.T) {
	dir := tempDir(t)

	if r := runLimen(t, dir, nil, "init"); r.code != 0 {
		t.Fatalf("init failed: %s", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".limen", "limen.yaml")); err != nil {
		t.Fatal(".limen/limen.yaml was not created")
	}
	// The scaffold must be readable by limen itself.
	if r := runLimen(t, dir, nil, "show"); r.code != 0 {
		t.Errorf("the scaffolded file does not load: %s", r.stderr)
	}
	if r := runLimen(t, dir, nil, "init"); r.code == 0 {
		t.Error("a second init must refuse rather than clobber identity")
	}
}

func TestCLIInitProtectsTheFileFromBeingCommitted(t *testing.T) {
	// The file carries per-machine identity and may end up holding a key, so the
	// ignore entry is part of creating it rather than something to remember.
	dir := tempDir(t)
	write(t, filepath.Join(dir, ".gitignore"), "target/\n")

	r := runLimen(t, dir, nil, "init")
	if r.code != 0 {
		t.Fatalf("init failed: %s", r.stderr)
	}
	if !strings.Contains(r.stdout, "added to .gitignore") {
		t.Errorf("init should report the ignore entry:\n%s", r.stdout)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), ".limen/limen.yaml") {
		t.Errorf(".gitignore lacks the entry:\n%s", body)
	}
	if !strings.Contains(string(body), "target/") {
		t.Error("existing .gitignore content must survive")
	}

	// Idempotent: a second init in a fresh dir with the entry already present
	// must not duplicate it.
	dir2 := tempDir(t)
	write(t, filepath.Join(dir2, ".gitignore"), "target/\n.limen/limen.yaml\n")
	r = runLimen(t, dir2, nil, "init")
	if !strings.Contains(r.stdout, "already in .gitignore") {
		t.Errorf("expected the already-present path:\n%s", r.stdout)
	}
	body, _ = os.ReadFile(filepath.Join(dir2, ".gitignore"))
	if strings.Count(string(body), ".limen/limen.yaml") != 1 {
		t.Errorf("entry duplicated:\n%s", body)
	}
}

func TestCLIInitWithoutGitignoreUsesGitInfoExclude(t *testing.T) {
	// Planting a .gitignore would add a tracked file to someone's repository in
	// order to hide an untracked one. .git/info/exclude does the same job and is
	// not repository content, so it is used whenever there is a work tree.
	dir := tempDir(t)
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	r := runLimen(t, dir, nil, "init")
	if r.code != 0 {
		t.Fatalf("init failed: %s", r.stderr)
	}
	if !strings.Contains(r.stdout, ".git/info/exclude") {
		t.Errorf("expected mention of the exclude file, got:\n%s", r.stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
		t.Error("init must not create a .gitignore")
	}
	if err := exec.Command("git", "-C", dir, "check-ignore", "-q", ".limen/limen.yaml").Run(); err != nil {
		t.Error("git does not consider .limen/limen.yaml ignored")
	}
}

func TestCLIInitOutsideAGitRepoSaysThereIsNothingToIgnore(t *testing.T) {
	// Not a repository, so there is nothing the file could accidentally be
	// committed to. Advice about .gitignore would be noise.
	dir := tempDir(t)
	r := runLimen(t, dir, nil, "init")
	if r.code != 0 {
		t.Fatalf("init failed: %s", r.stderr)
	}
	if !strings.Contains(r.stdout, "Not a git repository") {
		t.Errorf("expected the no-repository note, got:\n%s", r.stdout)
	}
	for _, unwanted := range []string{".gitignore", "info/exclude"} {
		if _, err := os.Stat(filepath.Join(dir, unwanted)); err == nil {
			t.Errorf("created %s outside a repository", unwanted)
		}
	}
}

func TestCLIHookOutputIsValidShell(t *testing.T) {
	dir := tempDir(t)

	bash := runLimen(t, dir, nil, "hook", "bash")
	if !strings.Contains(bash.stdout, "PROMPT_COMMAND") {
		t.Error("bash hook missing PROMPT_COMMAND")
	}
	// Syntax-check the bash hook with bash itself.
	script := filepath.Join(dir, "hook.bash")
	write(t, script, bash.stdout)
	if out, err := exec.Command("bash", "-n", script).CombinedOutput(); err != nil {
		t.Errorf("bash hook is not valid bash: %v\n%s", err, out)
	}

	zsh := runLimen(t, dir, nil, "hook", "zsh")
	if !strings.Contains(zsh.stdout, "add-zsh-hook chpwd") {
		t.Error("zsh hook missing the chpwd registration")
	}
	if _, err := exec.LookPath("zsh"); err == nil {
		zscript := filepath.Join(dir, "hook.zsh")
		write(t, zscript, zsh.stdout)
		if out, err := exec.Command("zsh", "-n", zscript).CombinedOutput(); err != nil {
			t.Errorf("zsh hook is not valid zsh: %v\n%s", err, out)
		}
	}

	if r := runLimen(t, dir, nil, "hook", "fish"); r.code == 0 {
		t.Error("an unsupported shell should be an error")
	}
}

func TestCLIUnknownCommandAndHelp(t *testing.T) {
	dir := tempDir(t)
	// The error speaks English like every other string — ADR-0001 covers CLI
	// output, and the usage text it is paired with is English already.
	if r := runLimen(t, dir, nil, "nonsense"); r.code != 2 || !strings.Contains(r.stderr, "unknown command") {
		t.Errorf("unknown command: exit = %d, stderr = %q, want 2 and an English error", r.code, r.stderr)
	}
	if r := runLimen(t, dir, nil, "--help"); r.code != 0 || !strings.Contains(r.stdout, "limen show") {
		t.Errorf("help output unexpected: exit %d\n%s", r.code, r.stdout)
	}
	if r := runLimen(t, dir, nil, "--version"); r.code != 0 || !strings.Contains(r.stdout, version) {
		t.Errorf("version output unexpected: %s", r.stdout)
	}
}

func TestCLIDefaultsToShow(t *testing.T) {
	_, nested := project(t, fullConfig)
	bare := runLimen(t, nested, nil)
	explicit := runLimen(t, nested, nil, "show")
	if bare.stdout != explicit.stdout {
		t.Errorf("bare invocation differs from `show`:\n%q\nvs\n%q", bare.stdout, explicit.stdout)
	}
}

func TestCLIKeychainImportNamesWhatIsMissing(t *testing.T) {
	// Without a context there is nothing to import from.
	if r := runLimen(t, tempDir(t), nil, "keychain-import"); r.code == 0 || !strings.Contains(r.stderr, "no context") {
		t.Errorf("without context: exit %d stderr %q", r.code, r.stderr)
	}
	// With a context whose file carries no key, the command must refuse
	// rather than store an empty secret.
	_, nested := project(t, "label: x\n")
	if r := runLimen(t, nested, nil, "keychain-import"); r.code == 0 || !strings.Contains(r.stderr, "no plaintext key") {
		t.Errorf("without key: exit %d stderr %q", r.code, r.stderr)
	}
}
