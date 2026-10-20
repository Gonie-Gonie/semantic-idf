# 정적 HVAC 시스템과 서비스 경로

**HVAC** 탭은 입력의 설비, 루프와 노드 참조를 해석합니다. 어떤 모델
시스템이 Zone/Space를 공조하고 어떤 부품이 서비스를 전달하며 지원 루프가
어떻게 결합되는지 정적 서비스 모델로 만듭니다. 수리, 기류 또는 열역학
해석기를 실행하는 것은 아닙니다.

## 서비스 Zone 확인 순서 {#hvac-workflow}

1. 현재 분석이 준비되면 HVAC의 **Zone services**에서 시작합니다.
2. Zone/Space를 선택하고 냉방, 난방, 환기, 배기나 직접 서비스 경로를 봅니다.
3. **Service**, **Path**, **Medium** 필터로 관심 관계를 좁힙니다.
4. 전달 부품이나 공조원을 선택하고 로컬 상세를 읽습니다.
5. 관련 Air Loop, Plant Loop 또는 다른 루프에서 회로 구조를 확인합니다.
6. 미해석 참조/경고를 검토하고 원본 입력과 비교합니다.
7. 적합한 시뮬레이션을 실행한 뒤 실제 운영은 Simulation HVAC에서 확인합니다.

루프 선택기는 Air Loops, Plant Loops와 Other Loops를 구분합니다. Other에는
주요 두 그룹 밖의 지원 루프 역할을 포함합니다. Fit은 도면 범위를 바꾸고
Zone services는 개요로 돌아갑니다. Clear focus는 로컬 초점을 해제하고
Back/Forward는 로컬 HVAC 탐색을 복원합니다.

HVAC 선택은 Metrics/Profile/Topology의 공통 선택과 독립적입니다. 부품
선택은 원본을 자동으로 표시하거나 다른 결과 탭을 바꾸지 않습니다. 원본
편집이나 참조 검증은 표시된 객체 유형/이름을 확인하여 입력 필터로 찾습니다.

## 서비스, 경로와 매체 {#service-model}

| 개념 | 의미 |
| --- | --- |
| 서비스 대상 | 서비스 관계가 향하는 Zone 또는 Space |
| 서비스 종류 | 냉방, 난방, 환기, 배기, 직접 서비스와 지원 특수 유형 |
| 경로 유형 | 중앙 공기, 직접 Zone, 수배관, 냉매, 복사 또는 로컬 전달 방식 |
| 공조 부품 | 경로에서 공조 역할을 공급하는 설비 |
| 전달 부품 | 서비스 대상에 전달하는 말단 또는 Zone 장비 |
| 전달 래퍼 | 말단과 별도로 보존하는 AirDistributionUnit 같은 참조 객체 |
| 결합 | 시스템, 루프 또는 다른 물리 매체 사이의 지원 관계 |

매체 필터는 공기, 냉수(CHW), 온수(HW), 응축수(CW), 냉매, 전기, 연료와
급탕/서비스 물을 포함합니다. 한 경로에 여러 매체가 있을 수 있습니다.
물 코일은 공기와 물 사이에서 열을 전달하며 동일 설비가 양쪽 역할을
가진다고 두 흐름을 하나의 노드로 합치지 않습니다.

경로는 원본 시스템과 해석된 AirLoop, PlantLoop, CondenserLoop 또는
냉매 시스템을 기록합니다. 공조, 전달과 지원 결합 참조를 따로 보존합니다.
상류 관계가 비었다고 로컬 설비에 중앙 열원을 임의로 붙이지 않습니다.

서비스 분류는 지원 객체의 역할과 참조를 따르며 표시 이름의 키워드만으로
판단하지 않습니다. “Cooling Pump” 같은 이름만으로 냉방 서비스나 물리
연결이 성립하지 않습니다.

## 루프 구조 복원 알고리즘 {#loop-algorithm}

루프 객체는 공급/수요 입출구 노드, BranchList와 ConnectorList를 지정합니다.
BranchList를 Branch로 해석하고 각 Branch의 순서 있는 부품 유형/이름/
입출구 선언을 펼칩니다. 유형별 조회로 실제 참조 객체와 지원 노드 필드를
확인합니다.

Splitter와 Mixer는 병렬 분기와 합류를 정의합니다. 표시기는 이 선언으로
선행, 병렬과 후행 분기를 배치하며 압축 경로가 실제 공학 배관 배치의
추정은 아닙니다. 도면 위치와 연결선 길이는 물리 거리 의미가 없습니다.

AirLoop 수요 경로는 SupplyPath/ReturnPath 부품, ZoneSplitter/Mixer와
지원 Plenum을 따릅니다. Zone/Space EquipmentConnections는 EquipmentList,
공기/입구/배기/환기 노드와 NodeList 확장을 해석합니다. 지원되는 장비 순서와
냉난방 시퀀스 정보를 보존하지만 이는 운영 시간 분율이 아닙니다.

AirDistributionUnit은 말단을 참조합니다. 말단 출구가 ADU 출구와 Zone
입구에 맞는지, 입구가 AirLoop 수요 경로에 속하는지 확인합니다.
EquipmentList에 말단이 있다는 것만으로 완전히 일치한 노드 사슬과 같은
강도의 근거가 되지는 않습니다.

루프 간 관계는 같은 물 코일의 공기/Plant 문맥 등장이나 냉수 회로와
응축 회로를 연결하는 Chiller 같은 근거를 사용합니다. 공유 설비 식별자를
여러 표현에서도 보존합니다. 두 루프에 그려진 코일 하나는 별도 코일
두 대가 아닙니다.

규칙 기반 관계는 원본 객체/필드와 규칙 추적을 보존합니다. 누락이나
불일치는 경고로 남으며 그래프를 연결하려고 노드 이름을 조용히 수정하거나
추측 분기를 추가하지 않습니다.

## 장비 계열별 해석 {#equipment-families}

| 계열 | 확인할 정적 관계 | 중요한 경계 |
| --- | --- | --- |
| 물 코일을 쓰는 중앙 공기 | Zone 말단/ADU → AirLoop 공조 → 코일 PlantLoop | 공기 전달과 물 열원 회로는 별도 역할 |
| Fan coil/수배관 Zone 장비 | 로컬 팬/코일 전달과 해석된 물 루프 | AirLoop 없이 Plant 지원을 받을 수 있음 |
| PTAC/PTHP, Window AC, Unit heater | 장비 래퍼, 내부 코일/팬과 직접 Zone 서비스 | 자체 장비에 원격 Plant를 임의 추가하지 않음 |
| VRF 말단 | 말단 → 말단 목록 → 냉매 열원/시스템 | 여러 수신 Zone이 공유하는 실외기 식별이 중요 |
| Water-to-air/중앙 Heat pump | 지원 native 포트, 루프와 수신 참조 | 냉난방/회수 역할은 실제 포트와 연결에 의존 |
| 복사/복사대류 장비 | 서비스 Zone, 활성 표면/그룹과 물/전기 열원 | 표면 매개 전달은 직접 Zone-air 출력과 다름 |
| District 냉난방 | 구매 열원과 지원 루프/서비스 | 외부 열원은 현장 연소나 현장 압축기 전력이 아님 |
| IdealLoads | 선언된 직접 이상 공조 서비스 | 이상 전달 부하로 실제 장비 에너지 변환을 결정할 수 없음 |
| 환기/배기/ERV | 공기 노드, 실외/배기 경로와 팬/회수 부품 | 냉난방 서비스 없이 환기가 존재할 수 있음 |

지원 범위는 객체와 버전에 따라 다릅니다. 한 계열을 인식한다고 모든
변형, 확장 목록, 제어 전략이나 새 엔진 객체를 완전히 해석하는 것은
아닙니다. 경로, 부품과 루프 경고를 읽습니다. 서비스 없는 Zone/Plenum은
의도된 것일 수 있습니다. 누락 서비스는 추정 배정 대신 확인이 필요합니다.

## 설비 availability와 제어 확인 {#component-values}

루프 그림에서 설비를 선택하면 **Availability**, **On / Off 결정**,
**Parameter 제어**의 패널 세 개가 나옵니다. 공유 설비는 선택한 루프의
문맥으로 확인하므로
다른 회로의 setpoint를 해당 설비의 제어 목표로 임의 연결하지 않습니다.

누락되거나 지원되지 않는 설정은 미해석으로 남습니다.
오래된 분석 자료는 갱신된 availability·제어 화면을 표시하기 위해 원본 입력을 다시
분석해야 할 수 있습니다.

### Availability {#availability-definitions}

맨 왼쪽에는 각 availability 조건을 정의한 객체를 표시합니다. 관련
스케줄을 **ON / OFF** 시간 패치로 쌓고 맨 아래에 **최소 가동 허용 조건**을
표시합니다. 원본 스케줄은 양수이면 ON, 그 외에는 OFF입니다. 최종 행은
지원되는 scheduled-manager override를 포함한 최소 정적 설정 조건을
조합하므로 모든 원본 플래그의 단순 AND가 아닙니다.

NightCycle의 운전 판단, 실행 중 온도·부하와 실제 장비 dispatch 판단은 이 화면에서
제외하며 정적 결과를 미확인으로 만들지 않습니다. 누락·중복 참조나
미해석 설정은 미확인으로 남습니다. 설비에 별도 availability 스케줄이
없으면 설비 자체 행에 **별도 스케줄 없음**과 제한 없는 패치를 표시합니다.
루프나 plant operation의
활성화 조건까지 없어지는 것은 아닙니다. 최소 가동 허용 조건은 정적 설정을
설명하며 실제 실행 제어와 설비 운전 여부는 시뮬레이션에서 확인합니다.

예를 들어 Chiller:Electric:ReformulatedEIR에는 자체 Availability Schedule
필드가 없으므로 설비 자체 행의 제한 없음 표시는 정상입니다.
PlantEquipmentOperationSchemes의 활성화 스케줄과 정확한 equipment-list
포함 관계는 정적 availability를 계속 제한합니다.
[EnergyPlus Chiller 설명](https://bigladdersoftware.com/epx/docs/24-2/input-output-reference/group-plant-equipment.html#chillerelectricreformulatedeir)과
[Plant operation 제어](https://bigladdersoftware.com/epx/docs/24-2/input-output-reference/group-plant-condenser-control.html)를 참고합니다.

**Rep. day**, **Rep. week**, **Annual** 중에서 선택합니다. 대표일·대표주는
현재 연도의 1월 1일부터 시작하고 대표 시작 요일은 월요일입니다.
연간 보기는 해당 연도의 일반 Gregorian 요일을 따릅니다. 시뮬레이션의
공휴일, daylight saving, runtime override는 반영하지 않습니다.

휠로 시간축을 확대·축소하고 드래그하여 이동합니다. 모든 행은 시간
범위와 커서를 공유하므로 hover하여 같은 시각의 상태를 비교합니다.
원본의 분 단위 전환 시각과 확인된 OFF·미확인 구간을 보존합니다.
세부 판단 로직은 **On / Off 결정**에서 확인합니다.

### On / Off 결정 {#on-off-decisions}

먼저 **운전 허용 조건**에서 manager override와 실행 중 plant 장비 선택을
포함한 전체 운전 허용 규칙을 확인합니다. Availability의 **최소 가동 허용 조건**보다
넓은 범위입니다. **가동 명령**은 여기에 설비 활성화 조건을 조합합니다.
예를 들면 `운전 허용 조건 AND 공기 유량 요청`입니다. **AND**는
모든 조건이, **OR**는 하나 이상의 조건이 충족되어야 한다는 뜻입니다.
스케줄 비교에는 원본 스케줄 이름을 표시합니다. 조건식은 설정된 제어
로직을 설명하며 시뮬레이션의 실제 ON/OFF 결과는 아닙니다.

반복되는 manager 규칙은 이름 있는 정의로 한 번 표시하고 해당 조건에서
참조합니다. 참조할 때마다 같은 규칙을 펼쳐 읽을 필요 없이 그 정의에서
원본 객체와 입력을 확인합니다. 예를 들어 NightCycle 정의에는 원본에
설정된 적용 스케줄, fan 스케줄, Control Type, 허용 편차와 cycling 설정을
표시합니다. 결과는 실행 중 zone 온도, thermostat 조건과 cycle 시간에도
영향받으므로 스케줄만으로 결과 상태를 결정할 수 없습니다.
**Manager 실행 중 판정**, **운전 장비 선택**, **실행 중 제어 판정**은 실행 중
입력이 필요한 부분입니다. 참조 누락, 미지원 정의와 재분석 필요는 별도로
표시합니다. **공기 경로 미확인**은 분석에서 연결 관계를 확정하지 못했다는
뜻이며 일반적인 실행 중 입력이 아닙니다.

**Manager 허용 조건**은 manager 상태를 설비의 운전 허용 여부로 바꾸며,
**Fan 허용 조건**은 fan 자체 스케줄과 manager override도 적용합니다.
Manager 상태는 다음과 같이 읽습니다.

| Manager 상태 | 운전 허용 |
| --- | --- |
| ForceOff | 허용하지 않음 |
| CycleOn | 허용 |
| CycleOnZoneFansOnly | Zone fan은 허용, central fan은 허용하지 않음 |
| NoAction | Fan 자체 스케줄을 따름. Manager만 확인하는 허용 조건은 추가로 제한하지 않음 |

NoAction은 manager가 개입하지 않는다는 뜻입니다. Fan override는 자체
스케줄이 OFF여도 fan을 허용하거나 ON이어도 제한할 수 있으며 다른
허용 조건은 계속 적용됩니다. Air/zone manager는 **ForceOff →
CycleOnZoneFansOnly → CycleOn → NoAction**의 우선순위를 사용합니다.
Plant manager는 선언된 목록 순서에서 첫 non-NoAction 결과를 사용합니다.
이름 있는 규칙은 이 의미를 유지하며 manager 상태를 일반 AND/OR로
조합하지 않습니다.

지원되는 능동 설비의 활성화 조건에는 원본과 의존 대상을 표시합니다.
연결된 부하·thermostat·dispatch 객체는 판단에 쓰이는 입력이며 자동으로
별도의 AND 조건이 되지 않습니다. Continuous 또는 기본 Pump 모드에서
유량 요청이 없다고 OFF로 단정하지 않으며 수동 부품에는 능동 ON/OFF
command가 없습니다. 미확인 입력은 미해석으로 남고 조건식 자료가 없으면
재분석이 필요합니다. 위의 Availability 타임라인은 계속 최소 정적 조건만
보여줍니다.

### Parameter 제어 {#parameter-control}

화살표를 따라 **입력 / 목표**, **비교식 / Controller**, **Control
command**, **설비 응답 / 출력**의 네 그룹을 읽습니다. 간결한 블록 하나가
원본 객체를 나타냅니다. 가까운 연결은 짧고 부드러운 경로를 사용하고,
주요 controller, command와 응답 블록은 연결 관계에 맞춰 나란히 배치합니다.
넓은 화면에서 긴 직접 입력은 위쪽, 길게 되돌아오는 참조와 feedback은
아래쪽 경로를 사용하며 방향 전환을 완만하게 표시합니다. 연결이나 블록에
마우스를 올리거나 키보드 포커스를 두면 관련 경로와 양끝 블록을 강조합니다.
블록을 선택하면 그림 아래에서 원본 필드, 포트, 구성 요소와 연결을
확인합니다. 상세 정보의 스케줄 요약은 연간 범위가 아니라 대표일을 설명합니다.

범례는 **신호**, **참조**, **측정 피드백**을 구분합니다. 측정 피드백에는
지원되는 비교식에 **+**로 들어가는 측정값도 포함합니다. 뒤로 이어지는
측정값·setpoint 참조는 원본 관계가 feedback으로 명시한 경우를 제외하면
참조로 표시합니다. **?**는 부호가 미확정이라는 뜻이며 연결 누락이나
확인된 positive feedback을 뜻하지 않습니다.

| 역할 | 의미와 예 |
| --- | --- |
| 외부 입력 | 선택한 설비 밖에서 전달되는 demand, 스케줄, setpoint 또는 입구 조건 |
| Controller / setpoint | 감지값을 비교하거나 목표값·지원 제어 규칙을 제공하는 참조 객체 |
| 비교식 | Actuator의 응답 방향과 구분하여 명시하는 residual 또는 비교 관계 |
| 제어변수 | 요청 물/공기 유량, 속도, capacity command처럼 조작하는 값 |
| 상태변수 | command, 입구 상태와 설비 모델에 의해 결정되는 출구 조건, 열전달률, 소비전력 |
| 외부 출력 | 선택한 설비 경계 밖으로 전달되는 command 또는 물리 출구 값 |

외부 포트는 선택한 설비/회로 문맥을 사용하고 해석된 포트의 선언된
node/field 식별을 유지합니다. 해당 회로 포트가 미해석이라고 다른 회로의
포트를 가져오지 않습니다. 물리 유로만으로 feedback controller가
연결되었다고 판단하지 않습니다. 성능 곡선과
설정 제한값은 상태 결정 관계를 설명하며 계산되는 모든 상태가 별도의
제어변수라는 뜻은 아닙니다.

그래프의 **제어변수** 역할은 조작 command를 뜻합니다. EnergyPlus
Controller:WaterCoil의 원본 **Control Variable** 필드는 출구 공기 온도처럼
감지하거나 조절하려는 물리량을 뜻합니다. 원본 필드는 이 의미를 유지하며
**Actuator Variable**이 조작하는 유량을 지정합니다.

지원 규칙으로 확인된 **+ / −** 부호만 표시합니다. 관계 이름과
부호를 함께 읽습니다. 비교 입력의 부호와 controller의 Normal/Reverse
Action은 서로 다른 제어 단계를 설명합니다. 미확인 부호가 positive
feedback이라는 뜻은 아닙니다. Feedback 참조는 순환을 반복해서 펼치지
않고 원본과 의미를 유지하며 그림의 배치 순서가 시뮬레이션 실행 순서는 아닙니다. 실제
운전 상태의 수치와 수렴은 Simulation HVAC와 실행 출력 변수로 확인합니다.

지원되는 Controller:WaterCoil 온도 비교에서 EnergyPlus는
`residual = 감지값 − setpoint`를 사용합니다. 감지값 입력은 **+**, 목표값
입력은 **−**이며 actuator의 반응 부호가 아닙니다. 원본 **Action** 필드는
조작 유량 증가에 대한 응답을 별도로 나타냅니다. 난방 응답은 Normal,
냉방 응답은 Reverse입니다.
[EnergyPlus controller 설명](https://bigladdersoftware.com/epx/docs/24-2/input-output-reference/group-controllers.html#field-action)과
[원본 residual 계산](https://github.com/NREL/EnergyPlus/blob/v24.2.0/src/EnergyPlus/HVACControllers.cc#L1072)을 참고합니다.

TemperatureAndHumidityRatio 제어는 온도와 maximum humidity ratio의 목표값을
분리합니다. 한 제공 객체가 확인되어도 다른 목표값까지 확인된 것은 아니며
누락된 목표값은 명시적인 미해석 외부 입력으로 남습니다. 지원되는 습도
setpoint-manager는 Control Variable 필드가 없어도 해당 객체 유형에 따라
목표 물리량을 구분합니다. 습도·복합 모드의 세부 비교나 선택이 해석되지
않으면 의존 관계의 비교 부호를 미확인으로 유지합니다.

### 예: 냉수 서비스 추적 {#chilled-water-example}

Zone VAV 말단에서 시작하여 ADU/출구-입구 근거를 확인합니다. AirLoop의
냉수 코일로 이동하고 해당 코일이 있는 냉수 PlantLoop 수요 분기를
찾습니다. 공급 Chiller와 응축 관계도 확인합니다. 이 과정은 모델 연결을
확인하며 연간 코일 부하, 펌프 전력이나 매 시간 충분한 유량을 증명하지
않습니다. 이 값은 실제 실행의 출력이 필요합니다.

## 누락되거나 오해하기 쉬운 관계 {#hvac-problems}

| 증상 | 확인 |
| --- | --- |
| 장비는 있는데 Zone 서비스가 없음 | EquipmentConnections/List 대상, 말단 래퍼와 입출구 사슬 |
| 분기/커넥터가 없음 | 정확한 BranchList/ConnectorList 이름과 참조 객체 존재 |
| Plant 관계가 없음 | 이름이 아니라 실제 물 포트와 수요/공급 분기 표현 |
| 한 Zone이 여러 시스템에 연결됨 | 서로 다른 유효 경로, 서비스 역할, 시퀀스와 공유 부품 표현 |
| 같은 부품이 반복 표시됨 | 다른 문맥에서 같은 정의를 참조하는지 확인 |
| 직접 Zone 장비에 AirLoop가 없음 | 로컬/수배관/냉매 전달이라면 정상일 수 있음 |
| 그래프는 연결되는데 실행 실패 | 엔진 노드 규칙, 제어, 사이징, 스케줄과 오류 출력 확인 |

경고는 선언 참조 실패, 지원 규칙 검사나 분석기 한계를 설명합니다.
불완전한 그림은 미지원 모델링을 반영할 수 있고 완전해 보이는 그림도
엔진 해석 성공을 증명하지 않습니다. Diagnose에서 규칙 문제와 한계를
구분하고 관계를 바꾸기 전에 원본 객체를 확인합니다.

## 안전 필드 편집과 출력 모니터 {#hvac-edits}

Edit 동작이 있는 부품은 임의 구조 변경이 아니라 지원하는 안전 필드
집합을 편집합니다. 가용성 스케줄, 지원 용량/제어 필드 등 표시된 필드는
객체/필드 식별을 유지합니다. 해당 필드의 단위, 수치 또는 Autosize 허용
형식을 확인합니다.

**Apply** 전에 **Preview**를 실행합니다. 미리보기는 원본 변경과 경고를
보여주며 잘못된 대상, 범위 밖 필드와 안전하지 않은 필드는 적용을 막습니다.
양식을 변경하면 이전 미리보기가 무효입니다. 적용 후 변경된 작업 문서를
다시 분석하며 디스크 보존은 별도 **Save**가 필요합니다.

지원되는 노드/부품의 **Add monitor**는 Output:Variable 요청을 만듭니다.
Key value, 변수 이름, 보고 빈도와 선택 스케줄을 확인합니다. 기존 일치
요청이 이미 모니터를 포함할 수 있습니다. 모니터 추가는 다음 실행 입력을
바꾸며 오래된 결과 번들에 관측을 생성하지 않습니다.

모니터 추천은 문맥상 편의 기능입니다. 실제 생성 가능한 변수는 엔진 버전,
장비와 유효 출력 변수 사전에 달려 있습니다. 누락 값을 0 운영으로
판단하기 전에 실행의 사용 가능한 출력 출처를 확인합니다.

## 시뮬레이션 비교와 기계용 내보내기 {#hvac-exports}

정적 HVAC와 Simulation HVAC는 다른 질문에 답합니다. 정적 분석은 선언
루프, 분기, 장비와 서비스 수신자를 제공합니다. Simulation은 선택한
기간의 보고된 운영, 가능한 온도/유량, 부하, 전력/연료와 보고 비율을
보여줍니다. 비교 전에 원본 모델과 범위를 일치시킵니다.

재현 가능한 기계 분석에는 `hvac-graph`가 규칙, 서비스 또는 결합 탐색
JSON을 `semantic-idf.hvac.graph.v1` 스키마로 내보냅니다.

```text
semantic-idf hvac-graph --graph service --format json model.idf
semantic-idf hvac-graph --graph rule --format json model.idf
```

Rule graph는 원본 기반 관계, Service graph는 수신자 전달, Coupling graph는
지원 시스템/매체 연결을 표현합니다. 설정된 경우 선택 Debug 기능으로
분석 그래프를 볼 수 있지만 일반 Zone-services/루프 사용에는 필요하지
않습니다.

내보낸 자료를 연결할 때 객체 식별, 표현 문맥, 매체와 역할을 보존합니다.
그래프 연결은 에너지 배분 알고리즘이 아닙니다. 공유 에너지 풀과 변환
수량은 Simulation/Energy Path 규칙을 따릅니다.
