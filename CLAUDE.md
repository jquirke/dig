# Claude Development Context

## Project Overview
This is the Uber dig dependency injection framework. Recent work added map value group support to complement existing slice value groups.

## Key Files to Review

### Core Implementation
- `param.go:638` - **CRITICAL TODO**: Decoration + map interaction concern
- `result.go` - Value group result handling with name tracking
- `container.go` - Key storage structure changes for map support
- `scope.go` - Scoping behavior with keyed group values

### Test Files
- `dig_test.go` - Main test suite with recent map value group tests
- `decorate_test.go:455` - Existing decoration + map test (limited coverage)
- `param_test.go` - Parameter validation tests including invalid key types
- `DECORATION_TEST_GAPS.md` - **READ FIRST** - Critical test coverage gaps

## Recent Development History

### Map Value Group Implementation (3 commits)
1. **48e086f** - "Support simultaneous name and group tags"
   - Removed mutual exclusivity between `dig.Name` and `dig.Group`
   - Enables providing values with both name AND group tags

2. **deaa0ab** - "Track the key of any named group objects"
   - Added infrastructure to track map keys (names) in value groups
   - Changed internal storage from `[]reflect.Value` to `[]keyedGroupValue`

3. **5158d37** - "Support map value groups"
   - **MAIN FEATURE**: Added `map[string]T` consumption of value groups
   - Names become map keys, enabling `map["name1"]value1` access patterns

### Test Coverage Added
- ✅ Interface types with map value groups
- ✅ Pointer types with map value groups
- ✅ dig.As integration with maps (works correctly)
- ✅ Invalid map key type validation (already covered)
- ❌ **CRITICAL GAP**: Decoration + map interaction (see DECORATION_TEST_GAPS.md)

## Current Status & Priorities

### ✅ Verified Working
- Basic map value group functionality
- Interface/pointer type support
- dig.As transformation with maps
- Mixed consumption (individual names + slices + maps)
- Error validation for invalid scenarios

### ❌ Critical Gaps Remaining
1. **Decoration System Interaction** (HIGH PRIORITY)
   - Location: `param.go:638` TODO comment
   - Issue: Unclear how decoration handles slice→map conversion
   - Risk: May return wrong types or lose map key information
   - Documentation: See `DECORATION_TEST_GAPS.md`

2. **Soft Groups + Maps** (MEDIUM PRIORITY)
   - Behavior verification needed for soft group consumption as maps

## Code Patterns & Usage

### Basic Map Value Group Pattern
```go
// Providing
c.Provide(func() int{return 1}, dig.Name("key1"), dig.Group("nums"))
c.Provide(func() int{return 2}, dig.Name("key2"), dig.Group("nums"))

// Consuming
type Params struct {
    dig.In
    NumMap map[string]int `group:"nums"`        // {"key1":1, "key2":2}
    NumSlice []int         `group:"nums"`        // [1, 2] (order undefined)
    Individual1 int        `name:"key1"`         // 1
}
```

### Key Implementation Details
- Only `map[string]T` supported (string keys required)
- Every map entry MUST have a name or validation fails
- Same providers can be consumed as slices, maps, or individual named deps
- Names become map keys: `dig.Name("foo")` → `map["foo"]value`

## Debugging & Development Notes

### Common Issues
- Missing names in map groups cause runtime errors
- Only string-keyed maps allowed (validation at param creation)
- Decoration system uncertainty (see TODO comment)

### Test Running
```bash
go test -run "TestGroups" -v          # All group tests
go test -run "map.*value.*group" -v   # Map-specific tests
go test -run "decorate.*map" -v       # Decoration + map tests
```

### Future Sessions
When working on this codebase:
1. **READ DECORATION_TEST_GAPS.md FIRST** if working on decoration
2. Review this file for recent context
3. Check the TODO comment at `param.go:638`
4. Understand that map value groups are a recent addition to existing slice infrastructure

## References
- Original issue: https://github.com/uber-go/dig/issues/380
- Fx feature requests: https://github.com/uber-go/fx/issues/998, https://github.com/uber-go/fx/issues/1036
- Branch: `dig_380`
- Base branch: `master`