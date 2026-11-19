# AVREDIT Modular Architecture Design

_**Note**: This is a suggestion of implementation of an approach to a modular TUI application. This can and should be challenged to be improved. The module architecture is important to do right_

## 1. Purpose

This document defines a modular architecture for **avredit**, enabling optional features such as:

- Mouse support
- Schema Registry integration
- Future extensibility (plugins, UI enhancements)

Goals:

- Zero overhead when features are disabled
- Clean separation of concerns
- Maintainable and scalable design

---

## 2. Architectural Overview

Avredit will follow a:

> Core Engine + Optional Modules

Core responsibilities:

- Schema editing
- Rendering base UI
- State management

Modules:

- Extend behavior
- React to events
- Add UI overlays

---

## 3. Module Interface

```go
type Module interface {
    Name() string
    Init(m *Model) tea.Cmd
    Update(msg tea.Msg, m *Model) (tea.Model, tea.Cmd)
    View(m *Model) string
}
```

Design notes:

- Modules do not own the app state
- Modules act on the shared Model
- Modules are optional and dynamically loaded

---

## 4. Core Model Changes

```go
type Model struct {
    modules []Module
    config  Config

    // Existing state
    Schema        *Schema
    SelectedField int
    ScrollOffset  int

    Regions []Region
}
```

---

## 5. Configuration

Example config.json:

```json
{
  // ... Truncated
  "modules": {
    "mouse": {
      "enabled": true
    },
    "schemaRegistry": {
      "enabled": true
      "connections": [
        "prod-registry": {...},
        "local": {...}
      ]
    }
  }
}
```

---

## 6. Module Loading

```go
func loadModules(cfg Config) []Module {
    var mods []Module

    if cfg.Features.Mouse.Enabled {
        mods = append(mods, NewMouseModule())
    }

    if cfg.Features.SchemaRegistry.Enabled {
        mods = append(mods, NewSchemaRegistryModule())
    }

    return mods
}
```

---

## 7. Program Initialization

```go
opts := []tea.ProgramOption{}

if cfg.Features.Mouse.Enabled {
    opts = append(opts, tea.WithMouseCellMotion())
}

p := tea.NewProgram(model, opts...)
```

---

## 8. Update Flow

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    for _, mod := range m.modules {
        _, cmd := mod.Update(msg, &m)
        if cmd != nil {
            cmds = append(cmds, cmd)
        }
    }

    // Core update logic here

    return m, tea.Batch(cmds...)
}
```

---

## 9. View Composition

```go
func (m Model) View() string {
    var out strings.Builder

    out.WriteString(m.coreView())

    for _, mod := range m.modules {
        out.WriteString(mod.View(&m))
    }

    return out.String()
}
```

---

## 10. Region-Based Interaction Model

```go
type Region struct {
    X1, Y1 int
    X2, Y2 int
    ID     string
}
```

Purpose:

- Maps screen coordinates to logical UI elements
- Enables click handling

---

## 11. Mouse Module Example

```go
type MouseModule struct{}

func NewMouseModule() *MouseModule {
    return &MouseModule{}
}

func (m *MouseModule) Name() string { return "mouse" }

func (m *MouseModule) Update(msg tea.Msg, model *Model) (tea.Model, tea.Cmd) {
    mouse, ok := msg.(tea.MouseMsg)
    if !ok {
        return *model, nil
    }

    switch mouse.Type {
    case tea.MouseLeft:
        // Hit-testing logic
    case tea.MouseWheelDown:
        model.ScrollOffset++
    case tea.MouseWheelUp:
        model.ScrollOffset--
    }

    return *model, nil
}
```

---

## 12. Performance Considerations

To ensure zero overhead:

- Modules are only created if enabled
- No global flags scattered across code
- No execution of disabled modules

Result:

- Memory usage = minimal
- CPU usage = proportional to active modules

---

## 13. Future Evolution

Potential enhancements:

### 13.1 Event Bus

```go
type Event struct {
    Type string
    Payload any
}
```

### 13.2 Component System

- Encapsulated UI blocks
- Independent rendering and input handling

### 13.3 Plugin System

- Load modules dynamically
- External contributions possible

---

## 14. Summary

This architecture enables:

- Clean feature toggling
- Strong separation of concerns
- Extensibility for future features
- Minimal performance overhead

---

## 15. Recommendation

Implement incrementally:

1. Add module system
2. Extract mouse support into module
3. Introduce region-based interaction
4. Add schema registry module later
