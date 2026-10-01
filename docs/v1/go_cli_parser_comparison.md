# Go CLI Argument Parsing Options – Compact Analysis

## Overview

Comparison of leading Go CLI argument parsing libraries with focus on bloat, usability, and suitability for TUI applications like avredit.

---

# Options Summary

## Cobra

<https://github.com/spf13/cobra>
Full-featured CLI framework supporting subcommands, flags, help generation, and ecosystem integrations.
Designed for large, structured CLI applications with long-term scalability.

## Kong

<https://github.com/alecthomas/kong>
Lightweight, struct-based CLI parser with strong typing and minimal boilerplate.
Focused on simplicity, clean code, and minimal abstraction.

---

# Comparison Table

| Metric             | Cobra                     | Kong                     |
| ------------------ | ------------------------- | ------------------------ |
| Type               | Framework                 | Library                  |
| Approach           | Imperative command tree   | Declarative struct-based |
| Binary Size        | ~3.4MB (stripped)         | ~3.3MB (similar class)   |
| Runtime Overhead   | Negligible                | Negligible               |
| Code / Boilerplate | Higher                    | Lower                    |
| Mental Complexity  | Medium–High               | Low                      |
| Subcommand Support | Advanced, nested          | Supported via structs    |
| Help Generation    | Automatic, customizable   | Automatic                |
| Shell Completion   | Built-in                  | Limited                  |
| Config Integration | Strong (via Viper)        | Basic                    |
| Ecosystem          | Large, mature             | Smaller                  |
| Industry Adoption  | Very high                 | Moderate                 |
| Flexibility        | High                      | High                     |
| Learning Curve     | Medium                    | Low                      |
| TUI Integration    | Standard pattern          | Manual integration       |
| Scalability        | Excellent                 | Moderate–High            |
| Best Fit           | Large/multi-command tools | Small–medium tools       |

---

# Key Takeaways

- Binary size and runtime overhead are effectively equivalent.
- Primary differences are in code complexity and architectural structure.
- Cobra introduces more boilerplate but supports scaling and ecosystem integration.
- Kong minimizes code and abstraction but requires manual structuring for complex CLIs.

---

# Recommendation

| Scenario                         | Recommended Option |
| -------------------------------- | ------------------ |
| Minimal code / low bloat         | Kong               |
| Long-term growth / extensibility | Cobra              |
| Multi-command CLI with ecosystem | Cobra              |
| Simple or early-stage tool       | Kong               |
