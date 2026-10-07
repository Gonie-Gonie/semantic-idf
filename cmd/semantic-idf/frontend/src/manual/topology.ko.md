# 기하와 정적 열 Topology

Topology는 공간 기하와 모델에 선언된 열 관계를 함께 보여줍니다. 객체의
위치, 연결 대상과 기하/참조가 선언한 관계를 뒷받침하는지 확인하는 데
사용합니다.

## Topology 확인 순서 {#topology-workflow}

1. 현재 분석이 준비되면 **Topology**를 엽니다.
2. 전체 형상은 **3D**, 층 구성은 **Plan**, Zone 수준의 열 관계는
   **Network**를 선택합니다.
3. **Level**을 All 또는 특정 층으로 정합니다. 세 뷰가 같은 Level을 공유합니다.
4. 관심 Zone, 표면, 개구부 또는 연결을 선택하고 높이를 조절할 수 있는
   아래 상세 패널을 읽습니다.
5. 경계 규칙, 구성, 수량과 원본 관계를 확인합니다.
6. 문제는 **Tools / Diagnose**에서 확인하고 원본을 수정한 뒤 분석이
   갱신되면 다시 점검합니다.

Fit은 도면 범위를 맞추고 Expand는 도면 공간을 넓힙니다. 3D/Plan은
Zones, Surfaces, Openings 표시를 공유하며 Network에는 Metric과 Layout
설정이 있습니다. 표시 설정은 입력 모델을 바꾸지 않습니다.

Topology 활성 상태의 기본 키는 3D/Plan/Network 전환 `1`/`2`/`3`, Fit `F`,
Connectivity/Area/UA/QA 선택 `T`/`A`/`U`/`Q`입니다. 설정 가능한 단축키는
편집 입력을 가로채지 않습니다. Network 대상은 일정한 키보드 순서로
이동하여 Enter 또는 Space로 활성화할 수 있습니다.

## 공간 뷰와 좌표 규칙 {#spatial-coordinates}

3D는 외피 형상, 방위와 Zone/표면/개구부 배치를 보여줍니다. Plan은 선택한
층의 구성을 보여줍니다. 두 뷰는 같은 세계 좌표 폴리곤을 사용하며 선택한
Zone은 Network에서도 같은 식별자를 가집니다.

GlobalGeometryRules는 상세/직사각형 좌표계, 시작 꼭짓점과 입력 방향을
구분합니다. 지원되는 기존 직사각형 객체는 선언된 위치, 크기와 방위에서
폴리곤으로 확장합니다. 상세 표면은 명시 좌표 또는 지원되는 꼭짓점 수
해석을 사용합니다. 도면을 완성하기 위해 미해석 폴리곤을 임의 생성하지
않습니다.

상대 좌표에서는 Zone 원점을 Building North Axis로 회전하고 로컬 좌표는
Building North Axis + Zone Direction of Relative North로 회전한 뒤,
변환된 원점을 더합니다. 앱의 회전 행렬 R로 표현하면 다음과 같습니다.

```text
세계 꼭짓점 = R(건물 북축 + Zone 방향) × 로컬 꼭짓점
            + R(건물 북축) × Zone 원점
R(θ)(x,y,z) = (x cosθ - y sinθ, x sinθ + y cosθ, z)
```

세계 좌표 꼭짓점은 선언 그대로 사용하며 두 번 회전하거나 이동하지
않습니다. 방위각은 좌표 선언과 꼭짓점 법선 규칙에 맞춰 해석합니다.
모델이 돌아가 보이면 수정 전에 원본 North Axis와 상대 북방향을 확인합니다.

층은 지원되는 바닥 높이와 Zone 수직 문맥에서 추론합니다. 표시 그룹이며
추가 EnergyPlus Zone이나 건축 층 이름의 보장이 아닙니다. 높은 Zone,
중간층이나 경사 바닥은 특정 층 대신 All에서 확인할 필요가 있습니다.

도면에서 빠진 객체는 선택 층, 표시 설정, 미해석 소유 관계나 사용할 수
없는 기하 때문일 수 있습니다. 표시 제한을 해제하고 원본을 확인한 뒤
객체가 제거되었는지 판단합니다.

## 경계, 인터페이스와 연결 {#thermal-relations}

| 용어 | 의미 |
| --- | --- |
| 열 경계 | 한 열전달 표면의 소유자에서 선언 대상으로 향하는 기준 관계 |
| 열 인터페이스 | 상호 대응 Zone 간 표면 쌍을 하나로 취급한 인터페이스 |
| 열 연결 | 하나 이상의 경계/인터페이스를 포함한 소유자-대상 압축 엣지 |
| 기하 인접 | 모델링 의도를 검토하지만 열 연결을 만들지 않는 폴리곤 근거 |
| 공기 결합 | 표면 전도와 별도로 표현한 공기 이동 관계 |

표면의 **Outside Boundary Condition**과 참조 객체가 관계를 결정합니다.
Outdoors와 Ground는 해당 환경으로 연결됩니다. Foundation은 Kiva 대상을
해석합니다. Ground 전처리, OtherSideCoefficients와
OtherSideConditionsModel 계열은 고유한 경계 역할을 보존합니다.
Adiabatic은 명시적인 무열전달 경계이며 Network에서는 떨어진 선택 가능한
벽 표시로 나타납니다.

Surface 쌍은 서로를 참조하고 다른 Zone이 소유해야 합니다. 두 표면은
원본 탐색 대상으로 남지만 면적, UA와 연결 합계는 기준 인터페이스를
한 번만 셉니다. 내부 개구부 쌍도 한 번만 셉니다. 명확한 자기 참조는
단열 자기 관계가 될 수 있으며 그 외 잘못된 자기 참조는 QA 문제입니다.

선언된 Zone/Space 대상은 가상 대응 관계를 사용할 수 있습니다. 두 번째
상세 폴리곤이 실제로 있다는 근거는 아닙니다. 누락, 단방향 또는 중복
대응을 그래프 생성기가 고치지 않습니다. 잘못되거나 미해석인 관계는
QA에 남으며 일반 열 합계에 포함하지 않습니다.

세계 좌표에서 닿는 두 폴리곤이 모델에서 자동으로 열을 교환하지는 않습니다.
예를 들어 인접한 단열벽은 그대로 단열벽입니다. QA는 두 표면을 기하
관측으로 강조할 수 있지만 Zone 간 열 엣지를 만들지 않습니다.

## Network 지표와 선택 {#network-metrics}

Network는 Zone 수준 그래프입니다. 압축 Outdoors 끝점은 노출 방위를
전달하고 연결 상세는 소속 경계/개구부를 보존합니다. 벽/창마다 별도
그래프 노드를 만들지 않으며 경계 단계 내려가기나 Matrix 뷰를 제공하지
않습니다.

| Metric | 확인하는 질문 |
| --- | --- |
| Connectivity | 어떤 선언 열 관계가 소유자와 대상을 연결하는가? |
| Area | 관계가 나타내는 관련 경계 면적은 얼마인가? |
| UA | 구성과 면적 기반의 정적 열관류량이 있는가? |
| Exposure | 실외 방위와 일사/바람 노출이 어떻게 선언되었는가? |
| QA | 선언 규칙, 참조나 기하 중 무엇을 검토해야 하는가? |
| Air | 별도 모델링된 공기 이동 경로는 무엇인가? |

Spatial과 Network 배치는 같은 결정적 레코드를 사용합니다. 끝점을
드래그하면 표시 배치와 경로가 바뀌며 원본 좌표나 유량은 바뀌지 않습니다.
Zone 선택은 접속 연결과 한 단계 끝점을 강조합니다. 경계/개구부/연결
선택은 해당 관계를 강조합니다. 현재 Level의 무관한 객체는 희미해지며
모델에서 삭제되는 것은 아닙니다.

Topology는 Text/JSON/Table과 공통 원본 탐색 선택을 사용합니다. 선택을
위해 Level을 강제로 바꾸지 않습니다. 이력은 현재 캐시 보고서로 뷰,
층, 지표, 배치, 표시, 이동/확대와 안정적인 선택을 복원합니다. 선택 자체는
새 분석을 요청하지 않습니다.

## 면적, U와 UA 해석 {#topology-quantities}

Gross area는 개구부를 포함합니다. 불투명 면적은 해석된 개구부 면적을
빼고 음수가 되면 0으로 제한합니다. 개구부 초과 진단은 여전히 중요하며
0 제한이 불가능한 기하를 유효하게 만드는 것은 아닙니다.

```text
불투명 면적 = max(0, 총면적 - sum(개구부 면적))
불투명 UA = 불투명 면적 × 불투명 구성 U
개구부 UA = sum(개구부 면적 × 개구부 구성 U)
전체 UA = 불투명 UA + 개구부 UA
유효 면적 = 물리 인스턴스 면적 × 적용 승수 계수
```

기본 Network UI는 물리 Gross area와 UA를 보여주고 Multiplier를 별도로
표시합니다. 승수 적용 effective 필드는 보고서에 보존하며 집계 서명과
Batch Metrics의 고정 기준입니다. 기본 UI에 면적 기준 선택기는 없습니다.
승수는 반복 모델 인스턴스이며 폴리곤을 늘리는 것이 아닙니다. 수량을
대조할 때 Zone 계수뿐 아니라 표면/개구부 계수도 확인합니다.
현재 정적 Geometry/Topology의 유효 기하는 Zone.Multiplier를 읽으며
ZoneGroup의 ZoneList 승수를 펼쳐 적용하지 않습니다. Metrics는 그 그룹
계수도 포함합니다. ZoneGroup 모델에서는 표시된 계수를 확인한 뒤 유효
Topology 요약과 건물 Metrics 합계를 같은 기준으로 비교합니다.

예를 들어 물리 20 m² 벽에 창 4 m², 불투명 U=0.4, 창 U=2 W/(m²·K)이면
물리 UA는 16×0.4+4×2=14.4 W/K입니다. 적용 계수가 균일하게 3이면
유효 UA는 43.2 W/K, 총면적은 60 m²입니다. Zone 간 대응 표면 때문에
이를 다시 두 배로 만들지 않습니다.

완전한 UA에는 필요한 불투명/개구부 U가 있어야 합니다. 없는 U는 U=0인
재료가 아닙니다. Coverage는 관련 면적 중 열성능을 해석할 수 있는 비율을
기록하며 해석된 일부를 완전한 합계로 표시하지 않습니다. 구성 추정은
지원 재료/직접 필드를 따릅니다. 사용 전에
[구성 가정](./metrics.ko.md#construction-thermal)을 확인합니다.

정적 UA 단위는 W/K입니다. 열관류량이며 냉난방 부하, 현재 기상의 전력이나
연간 에너지가 아닙니다. Exposure는 선언 메타데이터이며 기상 의존 일사
획득이나 풍력 침기를 적분하지 않습니다.

## 공기 연결과 한계 {#air-coupling}

공기 레코드는 지원 ZoneMixing, ZoneCrossMixing, 냉동 도어 혼합,
Construction:AirBoundary, AirflowNetwork 표면/부품 경로와 해당 실외 환기
관계를 포함합니다. 제공되는 방향, 설계 유량/단위, 스케줄, 부품과 기초
표면을 확인합니다.

ZoneMixing은 방향성이 있으며 교차 혼합 등 양방향 규칙은 고유 의미를
보존합니다. 공기 엣지는 전도 엣지와 별도로 그리고 전도 행렬에서
제외합니다. m³/s의 설계 유량은 W/K나 kWh가 아닙니다. 스케줄,
압력/온도 효과와 엔진 결과 없이 실제 연간 교환을 결정하지 않습니다.

일반적인 Zone 로컬 침기는 임의의 Zone 간 엣지를 만들지 않습니다.
AFN 부품이나 참조 Zone/표면 누락은 해석 문제이며 0 유량의 증거가
아닙니다. [Profile 기류](./metrics.ko.md#profile-algorithms)에서 명목
설계 프로파일과 기상 의존 한계를 확인합니다.

## 기하 QA 알고리즘 {#geometry-qa}

기하 QA는 세계 좌표 평면 법선, 중심, 경계 범위, 폴리곤 면적과 양자화한
엣지 근거를 만듭니다. 거리 허용오차[m]는
`max(0.00001, 모델 경계 대각선 × 0.000001)`입니다. 보고서는 근거 해석을
위해 이 허용오차와 기하 규칙 버전을 보존합니다.

선언된 상호 대응 쌍은 다음을 확인합니다.

- 반대 법선: 내적 ≤ -0.99.
- 지원되는 분리 허용 범위 안의 평면 거리.
- 상대 면적 차이 ≤ 1%.
- 투영 폴리곤 겹침 비율 ≥ 0.99.

분리 허용 범위는 거리 허용오차의 10배에서 시작하며 해석된 구성 두께를
반영할 수 있습니다. 평면 거리 진단은 기하 거리 근거도 유지하므로
경고 하나만 보지 말고 통합 쌍 상태와 구성 문맥을 읽습니다.

겹침은 지배 평면에 폴리곤을 투영하여 클리핑하고 교집합 면적을 더 작은
폴리곤 면적으로 나눕니다. 높은 겹침 비율만으로 면적이 같음을 증명하지
않으므로 별도 면적 검사가 중요합니다. 불규칙/비볼록 기하와 허용오차
수준의 형상은 QA를 완전한 솔리드 증명으로 취급하지 말고 원본을 검토합니다.

Zone 외피 QA는 양자화한 무방향 엣지의 사용 수를 셉니다. 닫힌 셸은
엣지마다 두 표면이 사용하며 한 번은 열린 엣지, 두 번 초과는 비다양체입니다.
외피 체적은 공통 기하 중심 주위의 삼각형 사면체 기여를 합산합니다.
지원하는 외피 표현의 검사이며 모든 선언 EnergyPlus 체적의 대체나 임의
비볼록 솔리드의 정확한 해석기가 아닙니다. 계산/선언 차이가 10%를
초과하면 알립니다.

## Diagnose 문제 유형 {#topology-diagnostics}

| 유형 | 대표 코드 | 확인 내용 |
| --- | --- | --- |
| 경계/참조 | `missing_boundary_target`, `invalid_boundary_condition`, `surface_self_reference_invalid`, `surface_counterpart_missing`, `surface_counterpart_one_way`, `surface_counterpart_duplicate`, `surface_pair_zone_mismatch` | 경계 조건, 정확한 대상과 상호 소유 관계 |
| 구성/노출 | `surface_missing_construction`, `surface_construction_unresolved`, `boundary_exposure_rule_mismatch`, `surface_pair_construction_mismatch`, `surface_pair_layer_order_mismatch` | 구성 해석, 대응 층 순서와 일사/바람 규칙 |
| 쌍 기하 | `surface_pair_area_mismatch`, `surface_pair_plane_mismatch`, `surface_pair_normal_mismatch`, `surface_pair_overlap_mismatch` | 양쪽 세계 폴리곤, 입력 방향과 허용오차 |
| 개구부 | `fenestration_base_surface_missing`, `fenestration_zone_mismatch`, `fenestration_counterpart_missing`, `fenestration_counterpart_one_way`, `fenestration_area_mismatch`, `fenestration_area_exceeds_base`, `fenestration_construction_mismatch` | 기초 표면, 대응, 면적과 구성 |
| 외피 | `zone_shell_open`, `zone_shell_non_manifold`, `zone_volume_mismatch` | 열린/중복 엣지 위치와 선언 치수 |
| 공기 | `air_coupling_target_missing`, `airflow_network_surface_missing`, `airflow_network_component_missing` | 실제 공기 경로 원본과 참조 부품 |

동일 분석 문서의 Topology와 Diagnose는 같은 문제 식별자를 사용합니다.
기하 관측과 선언 규칙 문제는 구별합니다. 관측 선택은 두 원본 표면을
찾아줄 수 있지만 EnergyPlus 열 연결을 선언하지 않습니다. Topology
상세는 수량과 관련 객체를, Tools는 진단과 수정 동작을 제공합니다.

## 정적 Topology, 시뮬레이션과 내보내기 {#topology-exports}

Topology에는 시뮬레이션 열 지표, 기간 선택기나 열흐름 원장이 없습니다.
부호가 있는 관측 열흐름은 Simulation의 Heat Flow에서 확인합니다.
필요한 엔진 출력이 없어도 정적 그래프는 존재할 수 있습니다. 반대로
시뮬레이션 관측은 기준 모델 경계를 바꾸지 않습니다.

정적 보고서는 `semantic-idf.thermal-topology/v1`이며 안정적인 의미 ID,
원본 앵커와 모델 해시를 갖습니다. 시뮬레이션 오버레이는 별도 스키마와
출처/기간 정보를 사용합니다. 내보낸 자료에서도 이 구분을 보존합니다.

기본 Network 툴바에는 JSON 내보내기가 없습니다. CLI topology는 기준
JSON, GraphML과 DOT를 지원하며 level/metric/scope와 물리/effective
면적 기준을 선택할 수 있습니다.

```text
semantic-idf topology --level zone --metric ua --area-basis effective model.idf
semantic-idf topology --level boundary --metric qa --format graphml model.idf
```

Batch Metrics는 승수를 적용한 Topology 요약을 사용합니다. 수치 차이는
경계 범위와 열성능 커버리지가 맞을 때 의미가 있습니다. 원본 모델 해시가
다르면 다른 분석 모델입니다. 표시 이름이 같다는 이유만으로 시뮬레이션
값을 정적 그래프에 연결하지 않습니다.
