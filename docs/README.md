# Greg Documentation

This directory contains the official documentation for greg.

## Documentation Structure

```
docs/
├── dev/
│   ├── ARCHITECTURE.org    # ✅ CANONICAL - System architecture and design
│   └── CONTRIBUTING.org    # ✅ CANONICAL - Development guidelines
├── CONFIG.org              # ✅ CANONICAL - Configuration reference
├── PROVIDERS.org           # ✅ CANONICAL - Provider development guide
├── COMMANDS.org            # ✅ CANONICAL - Commands reference
├── BUILD_README.org        # Build instructions
├── QUICK_START.org         # Quick start guide
└── TROUBLESHOOTING.org     # Troubleshooting guide
```

## Root-Level Files

Some documentation files exist at the project root for visibility:

- **`README.org`** (root) - ✅ CANONICAL - Main user documentation
- **`CLAUDE.md`** (root) - ✅ CANONICAL - AI assistant guidance
- **`AGENTS.md`** (root) - ✅ CANONICAL - AI agent descriptions

## Canonical Locations

**Always refer to these locations for the most up-to-date documentation:**

| Topic | Canonical Location | Notes |
|-------|-------------------|-------|
| User Guide | `/README.org` | Main entry point |
| Architecture | `/docs/dev/ARCHITECTURE.org` | System design |
| Contributing | `/docs/dev/CONTRIBUTING.org` | Dev guidelines |
| Configuration | `/docs/CONFIG.org` | Config reference |
| Providers | `/docs/PROVIDERS.org` | Provider guide |
| Commands | `/docs/COMMANDS.org` | Command reference |
| AI Guidance | `/CLAUDE.md` | For AI assistants |
| AI Agents | `/AGENTS.md` | Agent descriptions |

## Deprecated/Legacy Files

The following files in the root directory may be outdated or duplicates:

- `ARCHITECTURE.org` (root) - Use `/docs/dev/ARCHITECTURE.org` instead
- `CONFIG.org` (root) - Use `/docs/CONFIG.org` instead
- `CONTRIBUTING.org` (root) - Use `/docs/dev/CONTRIBUTING.org` instead

**Note:** These root-level duplicates should be removed or redirected to canonical locations.

## Contributing to Documentation

When updating documentation:

1. **Always edit the canonical version** (see table above)
2. Run `/docs` command to synchronize documentation
3. Check for drift: `just lint` includes doc checks
4. Update this README.md if structure changes

## Documentation Format

- **`.org` files** - Emacs Org mode format (primary)
- **`.md` files** - Markdown format (secondary)

Most greg documentation uses `.org` format for better structure and features.

## See Also

- [Quick Start Guide](QUICK_START.org)
- [Troubleshooting](TROUBLESHOOTING.org)
- [Build Instructions](BUILD_README.org)
