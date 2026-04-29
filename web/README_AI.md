# Angular Dashboard (Open-Guard)

**Core Intent:** Admin UI for managing policies, IAM, connectors, and viewing threats.
**Tech:** Angular 19+ (Standalone Components, Signals).

**AI Rules:**
- We use Signals (`signal`, `computed`) heavily over RxJS/BehaviorSubjects.
- Files > 300 lines should be split into smaller standalone components.
- Do not dump the whole `web/` folder into context. Locate specific components via filename.
