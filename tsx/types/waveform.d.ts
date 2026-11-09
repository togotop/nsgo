// The waveform chart: base/waveform.Chart as TSX props. A TSX app holds
// its samples in a signal and passes them as data; a Go app pushes them
// through a base/waveform.Source instead.

import type { Accessor } from "./index";
import type { StyleProp, ViewStyle, YNode } from "./types";

export interface WaveformProps {
  /** One column of equally spaced samples per series. */
  data?: Accessor<number[][]> | number[][];
  names?: Accessor<string[]> | string[];
  height?: number;
  /** x is Unix seconds; the x axis picks round time intervals. */
  time?: boolean;
  /** Pin the view to the newest samples, a stretch of follow x units. */
  follow?: number;
  cursor?: boolean;
  legend?: boolean;
  measure?: boolean;
  label?: string;
  note?: string;
  class?: string;
  style?: StyleProp<ViewStyle>;
  testID?: string;
  onCursorChange?: (x: number) => void;
}

export declare function Waveform(props: WaveformProps): YNode;

/** nsgo/ui's form takes a Variant on top of the base props. */
export interface StyledWaveformProps extends WaveformProps {
  variant?: "neutral" | "accent";
}

declare module "nsgo/ui" {
  export function Waveform(props: StyledWaveformProps): YNode;
}
