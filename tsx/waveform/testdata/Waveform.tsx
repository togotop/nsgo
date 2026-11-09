import { mount, signal, Text, View } from "nsgo";
import { Waveform } from "nsgo/base";

const columns = signal([[1, 2, 3, 4, 5, 4, 3, 2, 1]]);

function App() {
  return <View>
    <Text testID="add" role="button" onPress={() => columns.set(columns().concat([[9, 8, 7]]))}>Add</Text>
    <Waveform testID="wave" data={columns()} names={["CH1"]} xFormat="T{v}"
      cursor legend measure height={240} />
  </View>;
}
mount(App);
