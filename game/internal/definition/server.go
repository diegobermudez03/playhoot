package definition

type State map[string]string

type GameServer struct {
	GlobalState State
	Players     map[string]State
}

type ActiveFlow struct {
	Name          string
	State         State
	PlayersState  map[string]State
	CurrentStatus string
	EventsQueue   []string
}
