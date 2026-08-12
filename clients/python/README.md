# SemanticIDF Python topology client

`SemanticIDFClient.topology()` calls the local `/api/topology` RPC and returns the same
`ThermalTopologyReport` used by the desktop Topology view and CLI JSON export. GraphML
and DOT formats return text.

```python
from semantic_idf_client import SemanticIDFClient

report = SemanticIDFClient().topology(open("model.idf", encoding="utf-8").read())
print(report["schema"], len(report["connections"]))
```
