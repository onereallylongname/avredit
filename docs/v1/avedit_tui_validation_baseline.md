# AVREDIT – Competitive TUI Validation & UX Baseline Document

## 1. Purpose

This document defines the **minimum UX and validation expectations** required for avredit to be competitive with industry-standard editors (VSCode, IntelliJ), focusing specifically on:

- Schema validation UX
- Visual feedback (TUI equivalents of IDE affordances)
- Avoiding bloat
- Supporting validation-driven workflows

---

## 2. Core Principle

Avredit must not compete as a general editor.

> Avredit = Schema intelligence tool with editing capabilities

---

## 3. Baseline Expectations (Non-Negotiable)

These are the minimum features required to prevent immediate rejection by users.

### 3.1 Visual Feedback (TUI equivalents of IDE signals)

Implement the following patterns:

### ✅ Squiggles (Error Markers)

- Inline markers for invalid fields
- Highlight incorrect nodes in projection tree

### ✅ Red / Green Changes

- Red = breaking change or invalid state
- Green = safe / valid addition

Example:

    - field removed → RED
    + field added with default → GREEN

### ✅ Notifications Panel / Status Area

- Non-blocking alerts (compatibility break, validation error)
- Always-visible status bar summary

### ✅ Inline Validation Messages

- "Field type invalid"
- "Missing default for backward compatibility"

---

### 3.2 Editing Guarantees

Avredit must ensure:

- No invalid schema is silently produced
- Invalid states are visible immediately
- Prefer prevention over post-validation

---

### 3.3 Navigation

- Fast traversal of nested schema structures
- Clear focus indicator
- Predictable keyboard-driven interaction
- Predictable keyboard-driven interaction

---

## 4. Validation & Compatibility UX Requirements

### 4.1 Continuous Validation

- Validation runs after every edit
- Feedback is immediate and local

### 4.2 Compatibility Feedback

Must expose:

- Backward compatibility status
- Forward compatibility status
- Full compatibility status

### Example

    WARNING: Breaking change detected
    Reason: Removed required field 'id'

---

### 4.3 Change Explanation (Critical)

Each change must be classified:

    Change: AddField
    Impact: Safe
    Explanation: Field has default value

---

## 5. Anti-Bloat Guidelines

### 5.1 Explicitly Avoid

- Tabs (deferred/rejected)
- Complex layouts (panes, docking systems)
- Heavy mouse dependency
- IDE feature parity attempts

### 5.2 Avoid at Current Stage

- Multi-format support expansion
- Visual complexity beyond necessity

---

## 6. Integration Strategy

## Status: Planned (not required for initial validation work)

Target integrations:

- Schema Registry module
- Mouse support module

---

## 7. Failure Scenario Mitigation

## Problem

Users may disengage if:

- No immediate value is visible
- UX is unclear

## Mitigation: Guided Interaction

If TUI is idle for long:

### ✅ Show contextual hints

Example:

    Tip: Press 'a' to add a field
    Tip: Press 'v' to validate schema
    Tip: Press 'd' to delecte field
    Tip: Press ':diff' to start diff mode

### ✅ Suggest likely actions

Based on context:

- On new schema → suggest adding fields
- On detected diff → suggest validation

---

## 8. Competitive Positioning

## Do NOT compete with IDEs on

- General editing experience
- Ecosystem richness

## Compete on

- Schema correctness guarantees
- Compatibility reasoning
- Structured editing safety

---

## 9. Implementation Checklist

## Phase 1 – Validation UX

- [ ] Schema parsing integration
- [ ] Inline error highlighting ("squiggles")
- [ ] Status bar feedback

## Phase 2 – Compatibility

- [ ] Rule engine
- [ ] Real-time compatibility status
- [ ] Change classification

## Phase 3 – Interaction

- [ ] Contextual hints system
- [ ] Keyboard-driven workflows

## Phase 4 – Interaction 2

- [ ] Mouse workflows

---

## 10. Summary

To be competitive, avredit must:

- Match baseline feedback expectations (visibility of errors and status)
- Expose schema intelligence clearly and immediately
- Avoid UI bloat and IDE mimicry
- Guide user interaction to prevent confusion and drop-off
- Be non intrusive and allow breathing room for user to focus on:
  - Viewing and exploring schemas and fields
  - Editing schemas and fields
  - Managing schemas
