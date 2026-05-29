export const defaultSample = {
  name: "EnergyPlus RefBldgLargeOfficeNew2004 Chicago",
  path: "./samples/RefBldgLargeOfficeNew2004_Chicago.idf",
  source: "NREL/EnergyPlus testfiles/RefBldgLargeOfficeNew2004_Chicago.idf",
  sourceURL: "https://github.com/NREL/EnergyPlus/blob/develop/testfiles/RefBldgLargeOfficeNew2004_Chicago.idf",
};

const fallbackSampleIDF = `Version,
  24.1;                    !- Version Identifier

Building,
  Minimal Fallback Sample;  !- Name

Zone,
  Fallback Zone;            !- Name
`;

export async function loadDefaultSampleIDF() {
  try {
    const response = await fetch(defaultSample.path);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    return await response.text();
  } catch {
    return fallbackSampleIDF;
  }
}
