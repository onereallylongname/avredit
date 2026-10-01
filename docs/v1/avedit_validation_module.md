# AVREDIT Validation & Compatibility Module Design

## 1. Purpose

Define a validation and compatibility module for avredit enabling:

- Schema validation
- Version compatibility checks
- Semantic insight over schema changes

---

## 2. Module Scope

### 2.1 Schema Validation

Validate:

- JSON syntax
- Avro structural correctness
- Field/type definitions

### 2.2 Version Compatibility

Validate:

- Backward compatibility
- Forward compatibility
- Full compatibility

### 2.3 Diff

Diff is required for Compatibility and can be leveraged for users.
This should allow user to select a second schema from the file explorer or schema registry panes and present a float window with a diff representation.
It should support navigation and search as implemented on other cases.

### 2.4 Toggling Validation and Compatibility

Should the Validation module be active, the TUI should allow user to bypass (activate/deactivate) each validation individually.
The default should be all active.

- Validate:
  - `Enforce` : Enforce validations and don't allow user to commit invalid changes
  - `Warning`: Notify user of invalid fields and changes, but allow edit
  - `None`: Do not validate. Equivalent to disabled.
- Compatibility:
  - `Backward`: compatibility
  - `Forward`: compatibility
  - `Full`: compatibility
  - `None`: Do not validate. Equivalent to disabled.

#### How to toggle

This behaviors should be configurable on the configuration under the module configuration, and should be changeable for the current session via command:

- Validation: `:validation schema [Enforce|Warning|None]`
- Compatibility: `:validation compatibility [Backward|Forward|Full|None]`

As with other commands it should have auto complete and configuration.

---

## 3. Architecture Overview

```
Schema → Projection → Validation Module
                   → Compatibility Engine
```

Key principle:

- Operate on internal projection (not raw JSON)

---

## 4. Schema Validation Design

### 4.1 Flow

```
Input Schema
  → Parse (library)
  → Convert to Projection
  → Validate structural rules
```

### 4.2 Validation Levels

| Level      | Description                          |
| ---------- | ------------------------------------ |
| Syntax     | JSON correctness                     |
| Structural | Avro schema correctness              |
| Semantic   | Logical consistency (fields, unions) |

---

## 5. Compatibility Engine Design

### 5.1 Core API

```go
type CompatibilityResult struct {
    IsCompatible bool
    Type         string
    Issues       []Issue
}
```

### 5.2 Rules (initial set)

| Change                    | Result     |
| ------------------------- | ---------- |
| Add field with default    | Compatible |
| Add field without default | Breaking   |
| Remove optional field     | Compatible |
| Remove required field     | Breaking   |
| Type change               | Breaking   |

### 5.3 Process

```
Old Schema + New Schema
        ↓
Projection Diff
        ↓
Rule Evaluation
        ↓
Compatibility Result
```

---

## 6. Library Options

## hamba/avro

<https://github.com/hamba/avro>
Fast, actively maintained Avro parser and serializer for Go.
Preferred for schema parsing and validation.

## goavro

<https://github.com/linkedin/goavro>
Mature but less actively developed; codec-based parsing and validation.
Useful for decoding-based validation workflows.

---

## 7. Library Comparison

| Metric        | hamba/avro     | goavro                    |
| ------------- | -------------- | ------------------------- |
| Maintenance   | Active         | Maintenance mode          |
| Performance   | High           | Moderate                  |
| Ease of Use   | Clean API      | More verbose              |
| Validation    | Schema parsing | Parse + decode validation |
| Adoption      | Growing        | Historically widespread   |
| Binary Impact | Moderate       | Moderate                  |
| Complexity    | Medium         | Medium                    |

---

## 8. Design Decisions

- Use hamba/avro for parsing and schema validation
- Implement compatibility logic internally
- Use projection as the canonical representation

---

## 9. Implementation Plan

### Phase 1 – Validation

- Integrate schema parser
- Validate syntax and structure
- Surface errors in CLI/TUI
  - Notifications
  - Squiggles
  - Additional badge icon with warnings/error
  - Add warning/error to the details pane of the field

### Phase 2 – Diff Engine

- Compare projection snapshots
- Generate structured change list

### Phase 3 – Compatibility Engine

- Implement rule evaluation
- Support backward compatibility first

### Phase 4 – UX Integration

- CLI: avredit validate / diff
- TUI: live compatibility feedback

---

## 10. Risks

| Risk                     | Mitigation                      |
| ------------------------ | ------------------------------- |
| Incomplete spec coverage | Incremental rule implementation |
| Complexity of unions     | Isolate union handling logic    |
| False positives          | Provide detailed explanations   |

---

## 11. Summary

- Validation is supported by Go libraries
- Compatibility must be implemented manually
- Internal projection enables advanced features
- Combined module provides strong product differentiation
