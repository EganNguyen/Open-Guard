# Threat Service

**Core Intent:** Detects compromised sessions, impossible travel, and brute force via ClickHouse.
**Key Files:**
- `pkg/detector/brute_force.go`: Threshold based rules.
- `pkg/detector/impossible_travel.go`: GeoIP / Velocity based rules.

**AI Rules:**
- Read specific detector files to modify rules.
