# JTS testxml corpus

The XML test files under this directory are vendored verbatim from the
upstream JTS Topology Suite project:

- Source repository: https://github.com/locationtech/jts
- Source path: `modules/tests/src/test/resources/testxml/`
- Pinned to commit: see `.jts-commit-sha` in this directory.

JTS is dual-licensed under the Eclipse Public License 2.0 (EPL-2.0) and
the Eclipse Distribution License 1.0 (EDL-1.0). The EDL-1.0 (a BSD-3
variant) permits redistribution of these test fixtures alongside go-topology-suite,
which is itself MIT-licensed. The original copyright holders retain all
rights to the test data. See the upstream LICENSE files for full terms:

- EPL-2.0: https://www.eclipse.org/legal/epl-2.0/
- EDL-1.0: https://www.eclipse.org/org/documents/edl-v10.php

## Updating the vendored corpus

To update to a newer JTS commit:

1. Pick a new commit SHA from
   https://github.com/locationtech/jts/commits/master
2. Re-run the fetch flow (one-shot):

   ```sh
   SHA=<new-sha>
   BASE=https://raw.githubusercontent.com/locationtech/jts/$SHA/modules/tests/src/test/resources/testxml
   for sub in failure general misc robust validate; do
     for f in $(curl -sS "https://api.github.com/repos/locationtech/jts/contents/modules/tests/src/test/resources/testxml/$sub?ref=$SHA" \
                 | python3 -c 'import json,sys; [print(x["name"]) for x in json.load(sys.stdin) if x["name"].endswith(".xml")]'); do
       curl -sS "$BASE/$sub/$f" -o "internal/jtstest/testdata/upstream/$sub/$f"
     done
   done
   echo $SHA > internal/jtstest/testdata/upstream/.jts-commit-sha
   ```

3. Re-run `go test -tags=jts ./internal/jtstest/...` and inspect the
   pass/fail/skip deltas before committing.

## What the corpus contains

| Subdirectory | Files | Purpose                                          |
| -----------  | ----- | ------------------------------------------------ |
| `general/`   | 49    | Canonical conformance: predicates, relate, overlay. |
| `validate/`  | 9     | `isValid` / `isSimple` cases.                    |
| `robust/`    | 4     | Numerical-robustness stressors.                  |
| `misc/`      | 11    | Buffer, GEOS bug regressions, miscellaneous.     |
| `failure/`   | 5     | Inputs JTS itself fails on (precision-reduction, |
|              |       | adversarial buffers). Run for completeness;      |
|              |       | go-topology-suite's behaviour here is uncorrelated with JTS. |

Total: 78 XML files, ~2.9 MB.
