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

const defaultSampleTimeoutMS = 2500;

export async function loadDefaultSampleIDF() {
  const controller = typeof AbortController === "function" ? new AbortController() : null;
  let timeoutID = 0;
  try {
    const timeout = new Promise((_, reject) => {
      timeoutID = window.setTimeout(() => {
        controller?.abort();
        reject(new Error("Default sample load timed out"));
      }, defaultSampleTimeoutMS);
    });
    const response = await Promise.race([fetch(defaultSample.path, controller ? { signal: controller.signal } : undefined), timeout]);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    return await response.text();
  } catch {
    return fallbackSampleIDF;
  } finally {
    if (timeoutID) {
      window.clearTimeout(timeoutID);
    }
  }
}
