import { expect, open, test } from "nsgo/test";

test("renders a waveform and keeps drawing as samples arrive", async () => {
  const app = await open("Waveform.tsx");
  expect(app.shows("CH1")).toBe(true);
  expect(app.shows("T0")).toBe(true);
  expect(app.shows("Vpp 4")).toBe(true);
  await app.press("id:add");
  expect(app.shows("CH1")).toBe(true);
});
