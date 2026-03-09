## Summary

<!-- One-paragraph description focused on intent and risk -->

## Bead References

- Primary bead: `l3d.X`
- Additional beads: `l3d.X.Y` (if applicable)

## Acceptance Criteria Mapping

- [ ] AC-1 -> `TestName_Bead_l3d_X`
- [ ] AC-2 -> `TestName_Bead_l3d_X_Y`

## TDD Evidence

- [x] I wrote a failing test first
- [x] I added or updated regression tests
- [x] I verified the tests fail for the expected reason before implementation

## Test Evidence

- Changed test files:
  - `path/to/file_test.go`
- Commands run:
  - `go test -race ./...`

## Evidence Artifacts

- `bead-evidence/l3d.X.json`

## Reviewer Notes

- Risk areas:
- Rollback strategy:
