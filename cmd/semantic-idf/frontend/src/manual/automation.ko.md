# CLI·API·내보내기

실행 파일은 데스크톱과 비대화형 명령을 모두 지원합니다. 예제의 semantic-idf는 일반 이름이며, 실제 배포 실행 파일의 경로·버전 접미사로 바꿉니다.

## 실행과 입출력 {#invocation}

인수가 없으면 데스크톱 앱을 엽니다. cli 접두사 또는 인식되는 최상위 명령으로 자동화합니다.

```text
semantic-idf cli --help
semantic-idf cli metrics --help
semantic-idf version
semantic-idf cli metrics -format json -o metrics.json model.idf
```

모델 명령의 옵션은 입력 경로 앞에 둡니다. 공백이 있는 경로·객체 이름은 셸에 맞게 인용합니다. 성공은 종료 코드 0이며, 잘못된 인수·처리 실패는 1과 stderr의 Error 메시지를 반환합니다. 도움말은 정상 종료합니다.

지원되는 모델 명령에서 입력 `-`는 stdin, 출력 `-o -`는 stdout입니다. XLSX stdout은 바이너리이므로 텍스트로 재인코딩하는 셸 파이프라인 대신 바이너리를 보존하는 도구로 저장합니다. Energy Path의 모델·결과는 파일 시스템 경로가 필요합니다.

## 정적 분석 명령 {#analysis-commands}

| 명령 | 출력 형식 | 목적 |
| --- | --- | --- |
| metrics | text, json, csv, xlsx | 공통 정적 지표 정의·수치 |
| batch-metrics | csv, json, text, xlsx | 독립 모델 비교 |
| diagnostics | text, json, csv | Diagnose 문제 |
| analyze | json, text | 통합 분석 보고서 |
| topology | json, graphml, dot | 정적 열 연결 projection |
| hvac-graph | json, text | HVAC rule·service·coupling 탐색 |
| profile-graph | json, text | 해석된 Profile 시계열 |
| profile-qa | text, json, csv | Profile QA 문제·후보 |
| profile-schedules | json, text, csv | 해석된 스케줄·유사성 그룹 |

```text
semantic-idf cli metrics -format csv -o metrics.csv model.idf
semantic-idf cli batch-metrics -format xlsx -orientation files -o compare.xlsx a.idf b.epjson
semantic-idf cli diagnostics -format json -o issues.json model.idf
semantic-idf cli analyze -format json -o analysis.json model.idf
semantic-idf cli profile-schedules -format csv -o schedules.csv model.idf
semantic-idf cli hvac-graph -graph service -format json -o service.json model.idf
```

배치 방향은 metrics 또는 files입니다. HVAC 그래프는 rule·service·coupling입니다. 그래프 내보내기는 지원되는 모델 연결과 근거를 표현하며 시스템을 실행하지 않습니다.

기존 호환 명령에는 summary·multi-summary/multi-metrics·diagnose가 있습니다. 새 자동화는 목적을 분명히 하는 현재 명령 이름을 사용합니다.

## Topology projection 옵션 {#topology-export}

| 옵션 | 선택값 / 기본값 |
| --- | --- |
| -level | zone·boundary; 기본 zone |
| -metric | topology·area·ua·exposure·qa·air; 기본 topology |
| -scope | building·story·selection·neighbors; 기본 building |
| -area-basis | effective·physical; 기본 effective |
| -story | story 범위의 0부터 시작하는 층 인덱스 |
| -selection | selection/neighbors용 안정적인 entity ID |
| -neighbor-depth | 1–3; 기본 1 |
| -format | json·graphml·dot; 기본 json |

```text
semantic-idf cli topology -metric ua -area-basis physical -format graphml -o thermal.graphml model.idf
semantic-idf cli topology -scope story -story 0 -format json -o first-story.json model.idf
```

선택에는 내보낸 안정적인 entity ID를 사용합니다. 표시명을 확인 없이 대신 넣지 않습니다. 면적·UA·정규화된 양방향 경계·지원하지 않는 값은 [공간·열 연결](./topology.ko.md)을 참고합니다.

## 정리와 변환 {#clean-convert}

Clean은 적용 없이 미리볼 수 있습니다. 규칙은 default·all·none 또는 쉼표로 구분한 ID입니다. 제외 항목은 임의 객체명이 아닌 candidate key입니다.

```text
semantic-idf cli clean --dry-run -format json -o preview.json model.idf
semantic-idf cli clean -rules remove_unused_schedules,remove_duplicate_output_variables -o cleaned.idf model.idf
semantic-idf cli clean -rules none --compact -o compact.idf model.idf
semantic-idf cli clean -rules none --semantic-duplicates -o renamed.idf model.idf
semantic-idf cli convert -to idf -o model.idf model.epjson
semantic-idf cli convert -to json -o model.epjson model.idf
semantic-idf cli convert -to yaml -o model.semantic.yaml model.idf
semantic-idf cli convert -to table -o model.tables.xlsx model.idf
```

Semantic YAML은 보기용 내보내기이며, EnergyPlus 실행 입력을 손실 없이 대체하는 형식이라고 가정하지 않습니다. Table/XLSX 변환은 객체를 스타일이 있는 워크시트에 그룹화합니다. 앱의 JSON 입력 탭 선택은 파일 변환을 실행하는 것과 다릅니다.

기본·선택·미지원 정리 규칙은 [Tools](./tools.ko.md#cleanup-rules)에 있습니다. 검토 시 입력과 새 출력 위치를 구분합니다.

## 기존 Energy Path 읽기 {#energy-path-cli}

Energy Path는 기존 실행 디렉터리 또는 SQL을 읽습니다. EnergyPlus를 시작하거나 열린 모델을 편집하지 않습니다.

```text
semantic-idf energy-path -format json -o result.json C:/runs/office
semantic-idf energy-path -input C:/models/office.idf -scope zone -zone "Core_bottom" -period M1 -service cooling -format csv -include-trace -o january.csv C:/runs/office/eplusout.sql
```

| 옵션 | 의미 |
| --- | --- |
| -input | 일치하는 IDF/epJSON; 생략하면 검증 가능한 실행 입력 사용 |
| -scope | building(기본)·zone |
| -zone | zone 범위에서 정확한 Zone 이름 |
| -period | annual(기본)·M1–M12 |
| -service | all(기본)·cooling·heating; 현재 UI는 두 서비스를 함께 표시하지만 API/CLI는 선택 가능 |
| -format | json(기본)·csv |
| -include-trace | CSV에 source/link 행 추가; JSON은 항상 전체 표현 |
| -o / -output | 파일 또는 -로 stdout |

등록된 Energy Path 옵션은 위치 인수 앞뒤에 올 수 있습니다. 다른 명령의 기존 옵션 순서를 바꾸지는 않습니다. 필요한 경우 --로 옵션 처리를 종료합니다.

원본 모델은 보존된 SQL·실행 provenance와 일치해야 합니다. 가까운 위치·현재 편집기·비슷한 이름만으로 동등한 모델이 되지 않습니다. 출력 경로로 보호된 SQL·입력·실행 메타데이터를 덮어쓸 수 없습니다.

## JSON·CSV·XLSX·HTML 의미 {#export-meaning}

| 표현 | 용도 | 해석 |
| --- | --- | --- |
| Metrics JSON | 프로그램에서 그룹 분석 | 그룹별 메타데이터·상태 보존 |
| Metrics CSV | 스프레드시트·간단한 스크립트 | name,value; 이름에 [unit], 단위 없으면 [-] |
| XLSX | 검토·비교 표 | 서식 있는 워크북; 상태와 시트 의미도 확인 |
| Energy Path JSON | 공통 구조화 결과·선택 보기 | 보이는 ribbon뿐 아니라 전체 purposeResults·provenance 보존 |
| Energy Path 요약 CSV | 범위·기간·서비스 집계 | 선택 요약; trace 선택으로 원본·링크 추가 |
| Energy Path HTML/XLSX | 공유용 보고서 | 선택한 전체 서비스 표현; trace는 내보내기 설정 반영 |
| Topology GraphML/DOT | 그래프 도구 | 정적 projection, 시뮬레이션 흐름 장부와 구분 |

전체 JSON에는 현재 보이지 않는 기간·Zone도 포함될 수 있습니다. CSV를 모든 그래프의 손실 없는 대체물로 사용하지 않습니다. 표시 수치의 의미는 단위·상태·원본·분모에 달려 있습니다.

## 로컬 HTTP API {#http-api}

앱 로컬 API에 접근 가능할 때 파일 경로는 API 호스트의 경로입니다. 호출 측 SQL이 자동 업로드되지는 않습니다. 실제 접근 가능한 base URL을 설정해야 하며, 예시 포트만으로 서버 실행을 보장하지 않습니다.

POST /api/energy-path는 JSON 객체 하나를 받습니다.

```json
{
  "resultPath": "C:/runs/office/eplusout.sql",
  "inputPath": "C:/runs/office/model.idf",
  "scope": "zone",
  "zone": "Core_bottom",
  "period": "M1",
  "service": "cooling",
  "format": "json",
  "includeTrace": false
}
```

JSON은 공통 projection이고 format=csv는 UTF-8 CSV입니다. 잘못된 선택값·미등록 필드·여러 객체·64 KiB 초과 본문은 실패합니다. GET /api/metric-guides는 앱의 공통 지표 카탈로그를 반환합니다. POST /api/topology는 제공한 모델 텍스트를 지원 옵션으로 projection합니다.

다른 endpoint에는 설정·실행 계획·출력 검색·시뮬레이션·배치가 있습니다. 각자 요청 형식과 동작이 다르므로 결과 읽기를 실행 요청으로 해석하지 않습니다.

## Python과 재현 가능한 비교 {#python}

표준 라이브러리 기반 SemanticIDFClient는 HTTP와 Energy Path CLI stdout을 지원합니다. 배포된 clients/python 모듈을 import 가능하게 설정합니다. HTTP API가 없으면 CLI stdout을 사용할 수 있습니다.

```python
from semantic_idf_client import SemanticIDFClient

result = SemanticIDFClient.energy_path_stdio(
    "C:/tools/semantic-idf.exe", "C:/runs/office",
    input_path="C:/runs/office/model.idf",
    scope="zone", zone="Core_bottom", period="M1",
)
print(result["view"]["summary"])
```

두 방식 모두 Python에서 배분을 다시 계산하지 않고 공통 builder의 결과를 읽습니다. 실행 파일 버전·원본/실행 입력·SQL·엔진/기상·선택값을 비교 근거와 함께 유지합니다. 대응되는 단위·관측값을 비교하고 결측/null/0을 보존합니다. 내보내기를 파싱할 수 있다는 사실만으로 물리 모델이 승인되지는 않습니다.

## 개발 저장소 정리 {#repository-cleanup}

Windows의 소스 checkout에서 실행하는 개발 명령입니다. PowerShell에서는 `.\dev.bat`, 명령 프롬프트에서는 `dev`를 사용합니다. 저장소 정리는 개발 생성물을 관리하며, 위의 실행 파일 `cli clean` 명령은 모델을 편집합니다.

```text
.\dev.bat clean-repo -WhatIf
.\dev.bat clean-repo
.\dev.bat clean-repo --hard -WhatIf
.\dev.bat clean-repo --hard
.\dev.bat setup
```

`-WhatIf`로 파일 삭제 없이 정리 계획을 확인할 수 있습니다. 일반 `clean-repo`는 Go 빌드 캐시, 자동 생성된 프런트엔드 binding·빌드 파일과 확인된 임시 개발 생성물을 삭제합니다. `build/bin`, 설치된 Go/Wails 도구, 의존성 캐시, 로컬 시뮬레이션 근거와 재사용 비교 기준은 보존하므로 setup을 다시 실행할 필요가 없습니다. 다음 빌드·테스트에서 필요한 생성물이 다시 만들어집니다.

`clean-repo --hard`는 `build/bin`, `.runtime/`, `.tools/`도 삭제하여 도구 설치와 생성 상태를 처음 clone한 수준으로 초기화합니다. `.runtime/`의 로컬 시뮬레이션 capture와 baseline도 모두 삭제되므로 보관할 내용은 먼저 해당 디렉터리 밖으로 복사합니다. 다음 빌드·테스트 전에 `dev setup`을 실행합니다.

두 모드 모두 추적 소스, 유지 관리되는 빌드 아이콘, Git 메타데이터·hook과 명시적 생성 경로 밖의 모델·설정 파일을 보존합니다. 디렉터리 junction이나 symbolic link를 통해 checkout 밖의 파일을 삭제하지 않습니다.
