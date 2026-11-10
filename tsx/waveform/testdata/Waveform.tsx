import { mount, signal, Text, View } from "nsgo";
import { Waveform } from "nsgo/base";

const columns = signal([[1, 2, 3, 4, 5, 4, 3, 2, 1]]);

function App() {
  return <View>
    <Text testID="add" role="button" onPress={() => columns.set(columns().concat([[9, 8, 7]]))}>Add</Text>
    <Waveform testID="wave" data={columns()} names={["CH1"]} xFormat="T{v}"
      cursor legend measure height={240}
      divisionsX={10} divisionsY={8} timeBase={1}
      trigger={{ source: 0, level: 3, edge: "rising" }}
      measurements={[{ source: 0, kind: "vpp", label: "Vpp" }, { source: 0, kind: "frequency", label: "f" }]}
      channels={[{ offset: 0 }]} snap />
  </View>;
}
mount(App);
