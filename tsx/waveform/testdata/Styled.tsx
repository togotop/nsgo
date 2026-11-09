import { mount } from "nsgo";
import { Waveform } from "nsgo/ui";

function App() {
  return <Waveform testID="styled" data={[[1, 2, 3, 4, 5]]} names={["CH1"]}
    legend variant="accent" height={200} />;
}
mount(App);
