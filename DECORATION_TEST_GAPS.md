# Decoration Test Gaps for Map Value Groups

## Critical Gap: Your TODO Comment

**Location**: `param.go:638`
```go
// Check if we have decorated values
// qjeremy(how to handle this with maps?)
if decoratedItems, ok := pt.getDecoratedValues(c); ok {
    return decoratedItems, nil
}
```

## The Problem

The decoration system was originally designed for slice value groups. When map value groups were added, the decoration interaction may not have been fully considered.

### Key Concerns:

1. **Type Mismatch**:
   - `getDecoratedValues()` calls `c.getDecoratedValueGroup(pt.Group, pt.Type)`
   - `pt.Type` could be `[]T` (slice) or `map[string]T` (map)
   - Decorators might return slice types but consumers expect map types

2. **Inconsistent Return Types**:
   ```go
   // Scenario that may be broken:
   c.Provide(func() int{return 1}, dig.Name("a"), dig.Group("nums"))
   c.Provide(func() int{return 2}, dig.Name("b"), dig.Group("nums"))

   // Decorator expects and returns slice
   c.Decorate(func(nums []int) []int {
       return append(nums, 999)
   })

   // Consumer expects map - what happens?
   type In struct {
       dig.In
       NumMap map[string]int `group:"nums"`  // May get slice instead?
   }
   ```

3. **Missing Test Coverage**:
   - No tests combining decoration + map consumption
   - Unclear behavior when decorator returns slice but consumer wants map
   - No validation that map structure is preserved through decoration

## Potential Issues

### Issue 1: Wrong Return Type
Decoration may return `[]int{1, 2, 999}` when consumer expects `map[string]int{"a":1, "b":2}`.

### Issue 2: Lost Key Information
When decorating, the map keys (names) might be lost since decorators work with slices.

### Issue 3: Silent Failures
The system might fail silently or return unexpected data structures.

## Test Scenarios Needed

1. **Basic decoration + map consumption**:
   ```go
   // Provide with names
   c.Provide(..., dig.Name("key1"), dig.Group("test"))

   // Decorate (expects slice)
   c.Decorate(func(items []T) []T { return modified })

   // Consume as map
   map[string]T `group:"test"`
   ```

2. **Map-aware decoration**:
   ```go
   // Decorator that works with maps
   c.Decorate(func(items map[string]T) map[string]T { return modified })
   ```

3. **Mixed consumption**:
   ```go
   // One consumer wants slice, another wants map
   []T `group:"test"`
   map[string]T `group:"test"`
   ```

## Existing Map Decoration Test

There IS one existing test in `decorate_test.go:455` - "decorate with map value groups". However, this test:
- Uses `map[string]string` for both input AND output of decorator
- May not test the problematic slice→map conversion path
- Doesn't test mixed slice/map consumption scenarios

## CRITICAL DISCOVERY ⚠️

**ROOT CAUSE IDENTIFIED**: Slice decorators are fundamentally incompatible with named value groups.

### What Works ✅
- **Unnamed groups** + **slice decorators** + **slice consumers**
- **Named groups** + **map decorators** + **map consumers** (existing test: `decorate_test.go:455`)

### What's Broken ❌
- **Named groups** + **slice decorators** + **any consumers**

### The Problem
When you provide values with names (`dig.Name()`) and then use a slice decorator (`func([]T) []T`), the decorator strips away the key information needed to reconstruct maps. The slice decorator only sees `[value1, value2]` without knowing which value corresponds to which name.

### Evidence
```go
// This pattern is BROKEN:
c.Provide(func() int{return 1}, dig.Name("a"), dig.Group("nums"))
c.Provide(func() int{return 2}, dig.Name("b"), dig.Group("nums"))

c.Decorate(func(nums []int) []int {
    // This sees [1, 2] but has NO KNOWLEDGE of names "a", "b"
    return modified_slice
})

// Consumers get UNDECORATED values:
map[string]int `group:"nums"` // Gets original: {"a":1, "b":2}
[]int `group:"nums"`          // Gets original: [1, 2]
```

### Why This Happens
1. **Storage**: Decorated values stored under slice type `[]T`
2. **Lookup**: Map consumers look for type `map[string]T`
3. **Mismatch**: No decorated values found, falls back to original providers
4. **Result**: Both slice AND map consumers get undecorated values

## Resolution Priority

**CRITICAL** - This represents a fundamental design limitation where two major features (named value groups + slice decorators) are mutually incompatible.

## Recommended Actions

1. **Document the limitation**: Slice decorators don't work with named value groups
2. **Update TODO comment**: Explain the fundamental incompatibility
3. **Consider design options**:
   - Forbid slice decorators when names are present (breaking change)
   - Support map decorators as the primary pattern for named groups
   - Implement key-preserving slice decoration (complex)
4. **Update tests**: Add tests that demonstrate the limitation and expected behavior