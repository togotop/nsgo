import { expect, open, test } from "nsgo/test";

test("renders the styled waveform from nsgo/ui", async () => {
  const app = await open("Styled.tsx");
  expect(app.shows("CH1")).toBe(true);
});
