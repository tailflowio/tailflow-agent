# Code Conventions - Go Projects

## Core Principles

### Clarity > Conciseness
- Long and explicit names > short and ambiguous
- Self-documenting code > explanatory comments
- Avoid abbreviations except standard ones (HTTP, ID, URL, JSON, etc.)

### Early Returns (Mandatory)
```go
// ✅ GOOD - Flat, readable
func ProcessOrder(order *Order) error {
    if order == nil {
        return ErrNullOrder
    }
    if !order.IsValid() {
        return ErrInvalidOrder
    }
    if order.Total() > maxLimit {
        return ErrAmountTooHigh
    }

    return order.Execute()
}

// ❌ BAD - Nested, unreadable
func ProcessOrder(order *Order) error {
    if order != nil {
        if order.IsValid() {
            if order.Total() <= maxLimit {
                return order.Execute()
            } else {
                return ErrAmountTooHigh
            }
        } else {
            return ErrInvalidOrder
        }
    } else {
        return ErrNullOrder
    }
}
```

## Naming Conventions

### Files and Packages
- **Packages**: Always lowercase, no underscores (e.g., `telemetryagent`)
- **Files**: snake_case for files (e.g., `status_subscriber_command.go`)
- **Tests**: Same name as the file + `_test.go` suffix

### Variables and Functions
```go
// Local variables: camelCase, explicit English names
var subscriberStatus string
var remainingRetries int

// Exported variables: PascalCase in English
type BoxSubscriber struct {}

// Constants: PascalCase (preferred) or SCREAMING_SNAKE_CASE in English
const MaxRetries = 3
const DefaultTimeout = 30 * time.Second

// Acronyms: Uppercase at the start, Pascal otherwise
type HTTPServer struct {}           // ✅
type HttpServer struct {}           // ❌
func NewHTTPClient() {}             // ✅
func parseHTTPResponse() {}         // ✅
func sendHTTPRequest() {}           // ✅

// Explicit names > short names
var connectedUser *User             // ✅
var usr *User                       // ❌ too short
var u *User                         // ❌ too short

// Exception: short loops and standard short variables
for i := 0; i < len(items); i++ {}  // ✅ OK for index
for user := range users {}          // ✅ Full name preferred
ctx := context.Background()         // ✅ OK - Go convention
err := doSomething()                // ✅ OK - Go convention

// Unused variables/parameters: always use _
func Process(_ context.Context, data Data) error  // ✅ ctx unused
for _, item := range items {}                      // ✅ index unused
```

### Interfaces
- Name interfaces based on their behavior (e.g., `Service`, `Method`, `Repository`)
- Suffix `In` for fx dependency structs (e.g., `BoxStatusSubscriberIn`)
- Suffix `Out` for fx return structs (e.g., `BoxStatusSubscriberOut`)

### Unused Parameters and Variables
Always use `_` to explicitly indicate that a parameter/variable is ignored:

```go
// ✅ Parameter imposed by interface but unused
func (b CommandIn) Execute(ctx context.Context, _ parameters.Params) error {
    return b.service.DoSomething(ctx)
}

// ✅ Unused loop index
for _, item := range items {
    process(item)
}

// ✅ Ignored return value (caution: do not ignore errors!)
result, _ := strconv.Atoi("123")  // OK only if error is impossible

// ❌ Bad - Parameter declared but unused (lint warning)
func Execute(ctx context.Context, params parameters.Params) error {
    return b.service.DoSomething(ctx)  // params never used
}
```

## File Structure

### Standard Organization
```go
package packagename

// 1. Imports (grouped and ordered)
import (
    // Standard library
    "context"
    "fmt"

    // External dependencies
    "github.com/external/package"

    // Internal packages
    "github.com/internal/project/internal/..."
)

// 2. Constants and global variables (if necessary)
const (
    defaultPageSize = 100
)

// 3. Types (structs, interfaces)
type MyStruct struct {
    // ...
}

// 4. Constructors
func NewMyStruct(...) ... {
    // ...
}

// 5. Methods (grouped by receiver)
func (m *MyStruct) Method1() {
    // ...
}

// 6. Private helper functions
func helperFunction() {
    // ...
}
```

### Imports
- Group by category (stdlib, external, internal)
- Leave a blank line between groups
- Alphabetical within each group
- No unused imports (golangci-lint will detect them)

## FX Conventions (Dependency Injection)

### In/Out Structure
```go
type ComponentIn struct {
    fx.In  // Always first

    // Dependencies (alphabetical order recommended)
    ConfigService    config.Service
    DatabaseService  database.Service
    LoggerService    *slog.Logger
}

type ComponentOut struct {
    fx.Out  // Always first

    Component Component `name:"component_name"`  // Name tag for identification
}
```

### FX Constructors
```go
func NewComponent(in ComponentIn) (out ComponentOut, err error) {
    // Validation if necessary

    // Initialization
    out.Component = &componentImpl{
        config:   in.ConfigService,
        database: in.DatabaseService,
        logger:   in.LoggerService,
    }

    return out, nil
}
```

## Test Conventions

### Suite Structure
```go
type ComponentTestSuite struct {
    suite.Suite

    // Context (always first)
    context context.Context

    // Mocks (alphabetical order)
    fakeDatabase *fakedatabase.Service
    fakeLogger   *fakelogger.Logger

    // Component under test (always last)
    component ComponentIn
}
```

### Standard SetupTest
```go
func (s *ComponentTestSuite) SetupTest() {
    s.context = context.Background()

    // Initialize mocks (alphabetical order)
    s.fakeDatabase = fakedatabase.NewService(s.T())
    s.fakeLogger = fakelogger.NewLogger(s.T())

    // Initialize component
    s.component = ComponentIn{
        DatabaseService: s.fakeDatabase,
        LoggerService:   s.fakeLogger,
    }
}
```

### Test Naming
```go
func (s *ComponentTestSuite) TestMethodName_Success() {
    // Arrange
    s.fakeDatabase.EXPECT().Method().Return(nil).Once()

    // Act
    err := s.component.MethodName(s.context)

    // Assert
    s.NoError(err)
}

func (s *ComponentTestSuite) TestMethodName_WhenDatabaseFails() {
    // Arrange
    s.fakeDatabase.EXPECT().Method().Return(assert.AnError).Once()

    // Act
    err := s.component.MethodName(s.context)

    // Assert
    s.Error(err)
}
```

## Documentation Conventions

### Exported Function Comments (godoc)
**Principle**: Comment ONLY if the function name is not self-explanatory enough.

```go
// ❌ USELESS - The name says it all
// NewBoxStatusSubscriber creates the event processing command.
func NewBoxStatusSubscriber(...)

// ❌ USELESS - Obvious
// Execute inserts the event into the database and propagates it to the worker.
func Execute(...)

// ❌ USELESS - Repeats the name
// GetUser retrieves a user by ID.
func GetUser(ctx context.Context, id string) (*User, error)

// ✅ USEFUL - Explains non-obvious behavior
// ProcessedAllPages paginates recursively until an empty response.
// The API does not provide the total number of pages.
func ProcessedAllPages(ctx context.Context, token string, page int) error
```

**Strict rule**: If in doubt → DO NOT comment. Improve the name instead.

**Private functions**: NEVER comment (except for truly complex logic).

### Inline Comments - Almost Nonexistent!

**Principle**: Code must be self-documenting. 99% of code requires NO comments.

#### ✅ The ONLY cases where commenting is acceptable (1 line max)
```go
// 1. Non-obvious technical constraint
// API rate limit: 10 req/s
time.Sleep(100 * time.Millisecond)

// 2. Complex business rule
// Business rule: 24h timeout for IN_PROGRESS status
if status == InProgress && time.Since(updated) > 24*time.Hour {
    status = Completed
}

// 3. Temporary workaround with reference
// Workaround JIRA-12345
if response.Data == nil {
    response.Data = []Item{}
}
```

#### ❌ NEVER use separator/divider comments
```go
// ❌ NO section separators
// ---------- helpers ----------
// --- Validate tests ---
// ---------------------------------------------------------------------------
```

#### ❌ NEVER comment (99% of cases)
```go
// ❌ ALL these cases: ZERO comments
if len(items) == 0 {
    break
}

counter++

err := repo.Insert(ctx, data)

for i := 0; i < len(items); i++ {
    process(items[i])
}

// No comment for simple conditions
if user.IsAdmin() {
    return true
}

// No comment for early returns
if !order.IsValid() {
    return ErrInvalidOrder
}

// No comment for clear loops
for _, item := range items {
    process(item)
}
```

#### ✅ Solution: Self-Documenting Code
```go
// Instead of commenting, use explicit names
func ValidateEmailAddress(email string) bool {
    return emailRegex.MatchString(email)
}

func hasExceededTimeout(updated time.Time) bool {
    return time.Since(updated) > 24*time.Hour
}

// Use well-named variables
isAdmin := user.HasRole(RoleAdmin)
if isAdmin {
    return true
}
```

**Absolute rule**:
- If in doubt → DO NOT comment
- If the code is unclear → Refactor, do not comment
- A good name > a comment

## Error Handling Conventions

### Early Return for Errors
```go
// ✅ GOOD - Early return, linear code, English messages
func ProcessData(ctx context.Context, id string) error {
    data, err := repo.Get(ctx, id)
    if err != nil {
        return fmt.Errorf("failed to get data: %w", err)
    }

    if !data.IsValid() {
        return ErrInvalidData
    }

    err = service.Process(ctx, data)
    if err != nil {
        return fmt.Errorf("failed to process data: %w", err)
    }

    return nil
}

// ❌ BAD - Deep indentation
func ProcessData(ctx context.Context, id string) error {
    data, err := repo.Get(ctx, id)
    if err == nil {
        if data.IsValid() {
            err = service.Process(ctx, data)
            if err != nil {
                return fmt.Errorf("failed to process data: %w", err)
            }
            return nil
        } else {
            return ErrInvalidData
        }
    } else {
        return fmt.Errorf("failed to get data: %w", err)
    }
}
```

### Error Wrapping
```go
// ✅ Wrap with context (English messages)
if err != nil {
    return fmt.Errorf("failed to insert subscriber: %w", err)
}

// ✅ Propagate directly if no context to add
if err != nil {
    return err
}

// ❌ Non-English messages
if err != nil {
    return fmt.Errorf("échec insertion abonné: %w", err)  // ❌
}
```

### Custom Errors (Sentinels)
```go
// Names and messages in English (standard Go convention)
var (
    ErrInvalidToken  = errors.New("invalid authentication token")
    ErrNotFound      = errors.New("data not found")
    ErrQuotaExceeded = errors.New("quota exceeded")
)

// Naming: always prefix with "Err" + English PascalCase
var ErrInvalidFormat = errors.New("invalid format")  // ✅
var InvalidFormat = errors.New("invalid format")     // ❌ no "Err" prefix
var ErrFormatInvalide = errors.New("invalid format") // ❌ French name
```

## Log Conventions (slog)

### Messages in English
All log messages must be in English.

### Log Levels
```go
// Debug: Detailed debugging information
logger.DebugContext(ctx, "processing item", "id", itemID)

// Info: General information about execution flow
logger.InfoContext(ctx, "subscriber status updated", "virtport_id", id)

// Warn: Abnormal but manageable situations
logger.WarnContext(ctx, "retry attempt", "attempt", retryCount)

// Error: Errors requiring attention
logger.ErrorContext(ctx, "failed to process event", "error", err)
```

### Structured Attributes (Mandatory)
```go
// ✅ GOOD - Structured key-value pairs
logger.InfoContext(ctx, "operation completed",
    "duration_ms", duration.Milliseconds(),
    "items_processed", count,
    "success", true,
)

// ❌ BAD - Formatted messages (unstructured)
logger.Info(fmt.Sprintf("processed %d items", count))

// ❌ BAD - Non-English messages
logger.InfoContext(ctx, "opération terminée", "count", count)
```

### Early Return with Logs
```go
// ✅ GOOD - Log before early return
func Process(ctx context.Context, id string) error {
    data, err := repo.Get(ctx, id)
    if err != nil {
        logger.ErrorContext(ctx, "failed to retrieve data",
            "id", id,
            "error", err,
        )
        return err
    }

    // Continue processing...
    return nil
}
```

## Performance Conventions

### Preallocations
```go
// Preallocate slices when size is known
items := make([]Item, 0, expectedSize)

// Avoid repeated appends without capacity
var items []Item  // ✗
for ... {
    items = append(items, item)
}
```

### Defer and Performance
```go
// OK for most cases
defer cleanup()

// Avoid in high-performance loops
for i := 0; i < millionItems; i++ {
    defer doSomething()  // ✗ - defer accumulation
}
```

## Project-Specific Conventions

### Pagination
```go
// Standard pagination pattern
offset := 0
pageSize := 100

for {
    response, err := service.GetData(ctx, offset, pageSize)
    if err != nil {
        return err
    }

    // Process data

    // Exit condition
    if len(response.Data) == 0 {
        break
    }

    offset += pageSize
}
```

### Truncate and Reset
```go
// Always truncate before a full reset
err = method.Truncate(ctx)
if err != nil {
    return fmt.Errorf("failed to truncate: %w", err)
}
```

### Batch Operations
```go
// Use BulkInsert for bulk operations
err = method.BulkInsert(ctx, items)
if err != nil {
    return fmt.Errorf("bulk insert failed: %w", err)
}
```
