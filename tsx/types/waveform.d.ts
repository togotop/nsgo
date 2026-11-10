// The waveform chart: base/waveform.Chart as TSX props. A TSX app holds
// its samples in a signal and passes them as data; a Go app pushes them
// through a base/waveform.Source instead.

import type { Accessor } from "./index";
import type { StyleProp, ViewStyle, YNode } from "./types";

/** A trigger's edge and mode are named as strings, as their Go enums are. */
export interface WaveformTrigger {
  source?: number;
  level?: number;
  edge?: "rising" | "falling" | "either";
  mode?: "auto" | "normal" | "single";
  /** Where the edge sits across the view, in (0, 1]; 0 or 0.5 is the middle. */
  position?: number;
}

/** One automatic reading of a series. */
export interface WaveformMeasurement {
  source?: number;
  kind?: "vpp" | "max" | "min" | "mean" | "rms" | "frequency" | "period" | "duty" | "rise" | "fall" | "span" | "rate";
  label?: string;
  /** "view" reads the visible window, "markers" the A to B interval. */
  over?: "view" | "markers";
}

/** One series' vertical controls, in the order of data. */
export interface WaveformChannel {
  offset?: number;
  scale?: number;
  invert?: boolean;
  /** AC coupling removes the mean of what the view shows. */
  ac?: boolean;
}

export interface WaveformProps {
  /** One column of equally spaced samples per series. */
  data?: Accessor<number[][]> | number[][];
  names?: Accessor<string[]> | string[];
  channels?: Accessor<WaveformChannel[]> | WaveformChannel[];
  height?: number;
  /** x is Unix seconds; the x axis picks round time intervals. */
  time?: boolean;
  /** A template for the x axis' labels: "{v}" becomes the tick's value. */
  xFormat?: Accessor<string> | string;
  /** Pin the view to the newest samples, a stretch of follow x units. */
  follow?: number;
  /** x units per horizontal division; with divisionsX it sets the window. */
  timeBase?: Accessor<number> | number;
  divisionsX?: Accessor<number> | number;
  divisionsY?: Accessor<number> | number;
  /** Draw x on a logarithmic scale; the view's low end must be positive. */
  xLog?: Accessor<boolean> | boolean;
  cursor?: boolean;
  legend?: boolean;
  measure?: boolean;
  /** Names for the two measurement markers, as ["A", "B"]. */
  markLabels?: Accessor<string[]> | string[];
  /** Settle the cursor and markers on a nearby peak, trough or end. */
  snap?: boolean;
  trigger?: Accessor<WaveformTrigger | undefined> | WaveformTrigger;
  measurements?: Accessor<WaveformMeasurement[]> | WaveformMeasurement[];
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
