# Development Rules

## Core Principles

- **Early returns mandatory**: ALWAYS favor early returns (see conventions.md)
- **Self-documenting code**: Explicit names > comments
- **Minimal comments**: Avoid noise, comment ONLY if real added value
  - If the code is clear → NO comment
  - If necessary → 1 short line maximum
  - Explain the WHY (business/technical), NEVER the WHAT

## Test Rules

### Code Coverage
- **MANDATORY**: 100% code coverage for all packages
- Verify with `make test-coverage` before each commit
- No exceptions tolerated - all branches and conditions must be tested

### Test Structure
- **Mandatory pattern**: testify/suite for all tests
- One test file per source file (e.g., `command.go` → `command_test.go`)
- Name suites: `<FeatureName>TestSuite`
- Always include `SetupTest()` for initialization

### Mocks (Mockery)
- **Generation**: Always via mockery (`make gen-mocks`)
- **Configuration**: Add interfaces in `.mockery.yaml`
- **Assertions**: Use `.EXPECT()` with `.Once()`, `.Times(n)`, etc.
- **Logger**: ALWAYS mock `*slog.Logger` like any other dependency

### Test Naming
- Pattern: `Test<Feature>_<TestedScenario>`
- Examples:
  - `TestExecute_Success`
  - `TestExecute_WhenInsertFails`
  - `TestExecute_WhenTokenIsNil`

## Code Rules

### Dependency Injection (Uber FX)
- Use FX modules to organize providers (see `internal/fx/modules.go`)
- Constructors should return concrete types or interfaces as appropriate
- Logger (`*slog.Logger`) is provided as a singleton via FX

### Error Handling
- Never swallow errors - always propagate them
- Do not log AND return the error - choose one or the other

### Context
- Always pass `context.Context` as the first parameter
- Propagate context through all method calls
- Never create a new context without reason (except in test setup)

### Unused Parameters
- Always use `_` for unused parameters
- Explicitly indicates intent to ignore the parameter
- Avoids warnings and clarifies code

```go
// ✅ Good - Parameter explicitly ignored
func Execute(ctx context.Context, _ any) error {
    // ...
}

// ❌ Bad - Parameter declared but unused
func Execute(ctx context.Context, params any) error {
    // ...
}
```

## Validation Workflow (Mandatory)

```bash
# Loop until everything passes
make lint && make test
```
- If `make lint` fails → fix the issues then re-run
- If `make test` fails → fix the code/test then re-run
- Only commit WHEN both pass
- Coverage must be at 100%
- Zero tolerance for lint warnings
- All tests must pass

## Quality Rules (golangci-lint)

### Mandatory Linting
- `make lint` BEFORE each commit
- Configuration: `scripts/.golangci.yaml` (~80 linters enabled)
- **Zero tolerance**: No warnings accepted

### Critical Enabled Linters
- **errcheck**: All errors must be checked
- **govet**: Suspicious construction issues
- **staticcheck**: Advanced static analysis
- **gosimple**: Possible simplifications
- **ineffassign**: Inefficient assignments
- **unused**: Dead code (variables, functions, imports)
- **gofmt/gofumpt**: Strict formatting
- **goimports**: Organized imports
- **contextcheck**: Correct context propagation
- **errname**: Error naming (ErrXxx)
- **errorlint**: Correct error wrapping
- **exhaustive**: Exhaustive enum switches
- **nilerr**: Incorrect nil,err returns
- **bodyclose**: HTTP body closure
- **rowserrcheck/sqlclosecheck**: Correct SQL handling
- **sloglint**: Correct slog usage
- **testifylint**: Correct testify usage
- **wsl**: Whitespace logic (logical separation)
- **lll**: Max line length 140 characters
- **prealloc**: Slice preallocation
- **revive**: Additional style rules
- **stylecheck**: Standard Go style

### Code Review
- Always request a review before merging
- Verify test coverage in the PR
- Ensure `make test` and `make test-coverage` pass

### Documentation and Comments

**Rule**: Almost ZERO comments. Self-documenting code in English.

#### When to Comment (Very Rare!)
- ✅ Truly non-obvious technical/business constraint (1 line max)
- ✅ Temporary workaround with ticket reference
- ❌ NEVER comment APIs/functions whose name is clear
- ❌ NEVER repeat what the code does
- ❌ NEVER comment clear code
- ❌ DO NOT comment conditions, loops, early returns

**Strict principle**:
- If in doubt → DO NOT comment
- If the code is unclear → Refactor (better names, function extraction)
- A good function/variable name > 100 comments

## Anti-patterns to Avoid

### Tests
- ❌ NOT using testify/suite → ALWAYS use suite pattern
- ❌ NOT mocking `*slog.Logger` → ALWAYS mock it like any other dependency
- ❌ Using `time.Sleep()` → use `GOEXPERIMENT=synctest`
- ❌ Ignoring mock expectations errors
- ❌ Reusing mocks between tests → recreate in `SetupTest()`
- ❌ Tests without assertions → every test must verify something

### Code and Style
- ❌ Deep indentation → ALWAYS use early returns
- ❌ Non-English comments → ALL code in English
- ❌ Useless comments → comment the WHY, not the WHAT
- ❌ Short non-explicit variable names (except `i`, `j` in short loops)
- ❌ Using `panic()` → return an error
- ❌ Creating goroutines without context → always manage lifecycle
- ❌ Hardcoding configurations → use the config system
- ❌ Ignoring errors → ALWAYS check and handle
- ❌ Assignment in `if` condition → separate into two lines
  ```go
  // ❌ Bad
  if err := doSomething(); err != nil {
      return err
  }

  // ✅ Good
  err := doSomething()
  if err != nil {
      return err
  }
  ```

### Architecture
- ❌ Bypassing FX dependency injection
- ❌ Creating cyclic dependencies
- ❌ Mixing business logic and infrastructure
- ❌ Global mutability → prefer dependency injection

### Performance
- ❌ No preallocation → preallocate slices when size is known
- ❌ String concatenation in loops → use `strings.Builder`
- ❌ Defer in performance-critical loops → manage manually
- ❌ Unnecessary allocations → reuse buffers

## Best Practices

### Performance
- Use pools for frequently allocated objects
- Avoid unnecessary allocations in loops
- Profile with pprof if needed

### Security
- Never log sensitive data (tokens, passwords, PII)
- Validate all external inputs
- Use strict types to prevent injections

### Maintenance
- Keep functions short and focused (< 50 lines)
- Extract complexity into helper functions
- Prefer composition over inheritance
