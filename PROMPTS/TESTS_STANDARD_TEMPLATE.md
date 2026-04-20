Read this file first before generating or modifying any test cases.

This file is the source of truth for:
- test structure
- wording
- formatting
- naming style
- comment style
- positive/negative coverage style
- consistency of the test suite

Use this file as the fixed standard for writing tests.

Also read:
- SPEC.md
- REQUIREMENTS_CHECKLIST.md
- the current codebase
- existing test files in the repository, if present

Core objective:
Generate unit tests that are structurally consistent, readable, minimal, and behavior-focused.

Most important rule:
Existing repository test files, if present, are the style reference.
Follow their structure and wording closely.
Do not invent a new test style unless no prior test style exists yet.

Scope rules:
- Test only behavior that is actually implemented in the code.
- Do not write speculative tests for unimplemented behavior.
- Do not add integration, runtime, end-to-end, or external-system tests unless explicitly requested.
- Do not fake coverage for functionality that does not exist yet.
- Keep tests aligned to real code, not ideal future design.

Coverage rules:
- Include both positive and negative tests where meaningful.
- Cover required behavior first.
- Add edge-case tests only when they directly protect implemented behavior.
- Avoid unnecessary clutter and redundant tests.
- Prefer a small, strong test set over a large noisy one.

Required style:
- Keep tests small, focused, and readable.
- Prefer explicit and descriptive test names.
- Use table-driven tests only where they improve clarity.
- Keep helper naming consistent within the suite.
- Keep assertions straightforward and readable.
- Keep files gofmt-compatible.
- Keep tests beside the package they test unless a different repository pattern already exists.

Required comment format above every test function:

/*
TC-<AREA>-<NUMBER>
Type: Positive|Negative
Title: <short heading>
Summary:
<2-4 line plain-English summary of the test>

Validates:
  - <point 1>
  - <point 2>
  - <point 3 if needed>
*/

Comment rules:
- Preserve this structure exactly.
- Use simple, clear English.
- Keep wording consistent across the suite.
- Do not switch to a different documentation style.
- Do not omit the comment block.

Naming rules:
- Use `TestXxx...` Go naming.
- Keep test names explicit and behavior-oriented.
- Prefer names that describe what behavior is being validated.
- Keep TC numbering and area prefixes consistent within the suite.

Assertion rules:
- Assert behavior first.
- Prefer direct assertions unless a helper clearly reduces duplication.
- Do not assert internal trivia unless that internal detail is the actual behavior being tested.
- If the code returns typed errors, assert typed error fields only when that behavior exists in code.
- If the code returns plain errors, do not force typed error assertions.

Modification rules:
- Do not rewrite existing tests unless the actual behavior or public contract has changed.
- If an existing test must be modified, explain exactly why.
- Keep stable behavior tests unchanged.

Before writing tests:
Inspect the repository and preserve:
- TC naming pattern
- comment wording pattern
- helper naming pattern
- assertion pattern
- file organization pattern
- overall detail level

When identifying tests to add, consider:
- newly added public behavior
- newly added internal helpers
- validation behavior
- codec or serialization behavior
- lifecycle or state behavior
- error behavior
- positive cases
- negative cases
- directly relevant edge cases tied to implemented behavior

Do not include:
- speculative future-behavior assertions
- unrelated cleanup
- unnecessary refactors
- duplicate tests that only restate existing coverage
- tests for features not implemented in the code

Expected output:
1. Add or update the required unit tests.
2. Keep formatting clean and consistent with the suite.
3. After generating tests, provide a summary containing:
   - files added
   - files modified
   - positive cases covered
   - negative cases covered
   - existing tests changed and why
   - remaining gaps intentionally deferred

Final verification checklist:
- tests match repository suite style
- header comments match the required format exactly
- wording is consistent across the suite
- only implemented behavior is tested
- positive and negative coverage are both present where meaningful
- no future behavior was accidentally tested
- tests remain minimal but sufficient
