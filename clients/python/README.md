# SemanticIDF Python client

This standard-library client supports the local HTTP API and read-only Energy
Path CLI stdout. Add this directory to `PYTHONPATH` to import it.

## Energy Path

```python
from semantic_idf_client import SemanticIDFClient

result = SemanticIDFClient().energy_path("run", input_path="run/model.idf", period="M1")
result_from_cli = SemanticIDFClient.energy_path_stdio(
    "C:/tools/semantic-idf.exe", "run", input_path="run/model.idf", period="M1",
)
print(result["view"]["summary"])
```

Both transports return the shared GUI v2 `purposeResults` and selected `view`;
neither starts a simulation or reparses results in Python. Use
`output_format="csv"` for summary text and `include_trace=True` for source/link
detail. See [Energy Path CLI and Python](../../docs/energy-path-cli.md) for
scope/service choices, verified run inputs, file paths and error handling.

## Topology

`SemanticIDFClient.topology()` calls the local `/api/topology` RPC and returns the same
`ThermalTopologyReport` used by the desktop Topology view and CLI JSON export. GraphML
and DOT formats return text.

```python
from semantic_idf_client import SemanticIDFClient

report = SemanticIDFClient().topology(open("model.idf", encoding="utf-8").read())
print(report["schema"], len(report["connections"]))
```
