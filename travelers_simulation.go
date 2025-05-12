package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

const (
	Nr_Of_Travelers = 5
	Min_Steps       = 10
	Max_Steps       = 100
	Min_Delay       = 10
	Max_Delay       = 50
	Board_Width     = 5
	Board_Height    = 5
	Max_Attempts    = 10
	Num_Traps       = 5
)

const (
	Max_Squatters      = 2    // maksymalna liczba squatters na planszy
	SquatterIntervalMs = 1000 // co ile ms próbujemy wygenerować nowego squattera
	SquatterLifeTime   = 1500
)

type Point struct {
	x, y int
}

var traps map[Point]bool = make(map[Point]bool)

func generateTraps() {
	// Mapa 'traps' jest już zainicjalizowana jako make(map[Point]bool), czyli jest pusta.
	// Chcemy dodać 'Num_Traps' unikalnych lokalizacji, które są pułapkami.
	// Dostęp do traps[jakisPunkt] zwróci 'false', jeśli punktu nie ma w mapie (czyli nie jest pułapką).

	if Num_Traps <= 0 { // Jeśli nie ma pułapek do utworzenia
		return
	}

	// Dodawaj pułapki, dopóki nie osiągniemy żądanej liczby unikalnych pułapek.
	// Pętla będzie kontynuowana, jeśli wylosowany punkt jest już pułapką,
	// ponieważ len(traps) się nie zwiększy.
	for len(traps) < Num_Traps {
		p := Point{rand.Intn(Board_Width), rand.Intn(Board_Height)}
		traps[p] = true // Ustawia punkt jako pułapkę (lub nadpisuje, jeśli już był)
	}
}

// Komenda do serwera pola
type BoardCommand struct {
	action     string      // "reserve"/"release"/"squatterReserve"/"status"
	replyChan  chan int    // 0=free, 1=occupied-by-traveler, 2=occupied-by-squatter (lub: success=1/fail=0 dla reserve)
	travelerId int         // tylko dla logów/debugowania
	squatChan  chan string // tylko dla squattera
}

var symbols = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ")

// Kanały serwerów pól
var board [Board_Width][Board_Height]chan BoardCommand

func move(p Point, dx, dy int) Point {
	return Point{
		x: (p.x + dx + Board_Width) % Board_Width,
		y: (p.y + dy + Board_Height) % Board_Height,
	}
}

func startFieldServer(ch <-chan BoardCommand, printer chan string, i, j int) {
	var occupied bool = false
	var occType int = 0 // 0 = puste, 1 = traveler, 2 = squatter
	//var currentId int
	var squatterChan chan string

	pos := Point{i, j}
	isTrap := traps[pos]
	var trapLog []string

	// Jeśli to pułapka, zapisz początkowy ślad
	if isTrap {
		now := time.Now().UnixNano()
		trapLog = append(trapLog, fmt.Sprintf("%d %d %d %d %c", now, 2000+3*i+100*j, i, j, '#'))
		printer <- trapLog[len(trapLog)-1] // Wypisz ślad pułapki
	}

	for cmd := range ch {
		switch cmd.action {
		case "reserve":
			if !occupied {
				if isTrap {
					cmd.replyChan <- 10
					occupied = true
					occType = 1
				} else {
					occupied = true
					occType = 1
					cmd.replyChan <- 1
				}
			} else if occType == 2 && squatterChan != nil {
				// squatter obecny, spróbuj go wypędzić
				squatterChan <- "kick"
				// 	time.Sleep(400 * time.Millisecond) // daj mu czas
				// 	if !occupied {
				// 		occupied = true
				// 		occType = 1
				// 		cmd.replyChan <- 1
				// 	} else {
				// 		cmd.replyChan <- 0
				// 	}
				// } else {
				cmd.replyChan <- 0
			} else {
				// Zajęte przez innego podróżnika
				cmd.replyChan <- 0
			}
		case "squatterReserve":
			if !occupied {
				if isTrap {
					cmd.replyChan <- 10
					occupied = true
					occType = 1
				} else {
					occupied, occType = true, 2
					cmd.replyChan <- 1
				}

			} else {
				cmd.replyChan <- 0
			}
		case "setSquatterChannel":
			squatterChan = cmd.squatChan
		case "release":
			occupied, occType = false, 0
			cmd.replyChan <- 1
		case "status":
			if isTrap {
				cmd.replyChan <- 10
			} else {
				cmd.replyChan <- occType
			}

		}
	}

}

func simulateTraveler(id int, printer chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()

	var history []string
	recordEvent := func(pos Point, sym rune) {
		now := time.Now().UnixNano()
		history = append(history, fmt.Sprintf("%d %d %d %d %c", now, id, pos.x, pos.y, sym))
	}

	steps := rand.Intn(Max_Steps-Min_Steps+1) + Min_Steps
	pos := Point{rand.Intn(Board_Width), rand.Intn(Board_Height)}
	symbol := symbols[id-1%len(symbols)]

	// Rezerwacja pola startowego
	//buforowany kanal bo inaczej nie dziala :(
	var successfullyStarted bool = false
	var startPos Point
	for attempts := 0; attempts < Max_Attempts*Board_Width*Board_Height; attempts++ { // Ogranicznik prób
		startPos = Point{rand.Intn(Board_Width), rand.Intn(Board_Height)}
		reply := make(chan int, 1)
		board[startPos.x][startPos.y] <- BoardCommand{action: "reserve", replyChan: reply, travelerId: id}
		reservationResult := <-reply

		if reservationResult == 1 { // Sukces
			pos = startPos // Ustawienie aktualnej pozycji podróżnika
			recordEvent(pos, symbol)
			successfullyStarted = true
			break
		} else if reservationResult == 10 { // Pułapka na starcie
			// Podróżnik "ginie" na pułapce przy próbie startu.
			// Serwer pola pułapki sam loguje interakcję z pułapką.
			// Podróżnik powinien zalogować swoje pojawienie się i zniknięcie.
			now := time.Now().UnixNano()
			// Dodajemy dwa wpisy: pojawienie się i natychmiastowa "śmierć"
			history = append(history, fmt.Sprintf("%d %d %d %d %c", now, id, startPos.x, startPos.y, symbol))
			history = append(history, fmt.Sprintf("%d %d %d %d %c", now+1, id, startPos.x, startPos.y, rune(symbol-'A'+'a'))) // Mała litera = śmierć
			time.Sleep(2000 * time.Millisecond)                                                                               // chwilowe zajęcie pola
			// Zwolnienie pola
			release := make(chan int, 1)
			board[startPos.x][startPos.y] <- BoardCommand{action: "release", replyChan: release, travelerId: id}
			<-release // odbierz odpowiedź
			// Ślad śmierci – pułapka pojawia się ponownie
			history = append(history, fmt.Sprintf("%d %d %d %d %c", now+2, 2000+3*startPos.x+100*startPos.y, startPos.x, startPos.y, '#')) // Pułapka pojawia się ponownie
			for _, e := range history {
				printer <- e
			}
			return // Podróżnik kończy, jeśli startuje na pułapce
		}
		// Jeśli reservationResult == 0 (zajęte), pętla będzie kontynuowana, próbując innego pola
		time.Sleep(1 * time.Millisecond) // Małe opóźnienie przed kolejną próbą
	}

	if !successfullyStarted {
		// fmt.Printf("Traveler %d: Could not find a starting position.\n", id) // Opcjonalny log
		// Jeśli nie udało się wystartować, wypisz historię (prawdopodobnie pustą) i zakończ
		for _, e := range history {
			printer <- e
		}
		return
	}

	moves := []struct{ dx, dy int }{
		{-1, 0}, {1, 0}, {0, -1}, {0, 1},
	}

	deadlock := false
	for i := 0; i < steps; i++ {
		time.Sleep(time.Duration(rand.Intn(Max_Delay-Min_Delay+1)+Min_Delay) * time.Millisecond)

		moved := false
		for attempt := 0; attempt < Max_Attempts; attempt++ {
			dir := moves[rand.Intn(len(moves))]
			newPos := move(pos, dir.dx, dir.dy)

			//buforowany kanal bo inaczej nie dziala :(
			reserve := make(chan int, 1)
			board[newPos.x][newPos.y] <- BoardCommand{action: "reserve", replyChan: reserve, travelerId: id}
			reservationResult := <-reserve // Odczytaj raz
			if reservationResult == 1 {
				// zwolnij poprzednie pole
				release := make(chan int, 1)
				board[pos.x][pos.y] <- BoardCommand{action: "release", replyChan: release, travelerId: id}
				<-release // odbierz odpowiedź
				pos = newPos
				recordEvent(pos, symbol)
				moved = true
				break
			} else if reservationResult == 10 {
				release := make(chan int, 1)
				board[pos.x][pos.y] <- BoardCommand{action: "release", replyChan: release, travelerId: id}
				<-release // odbierz odpowiedź
				time.Sleep(30 * time.Millisecond)
				//smierc na pulapce
				// konczymy watek
				// Poprawka: Upewnij się, że logujesz śmierć na nowej pozycji
				recordEvent(newPos, rune(symbol-'A'+'a')) // Mała litera oznacza śmierć/zakleszczenieee
				time.Sleep(200 * time.Millisecond)        // chwilowe zajęcie pola
				// Zwolnienie pola
				board[newPos.x][newPos.y] <- BoardCommand{action: "release", replyChan: release, travelerId: id}
				<-release // odbierz odpowiedź
				// Ślad śmierci – pułapka pojawia się ponownie
				recordEvent(newPos, '#') // Pułapka pojawia się ponownie
				for _, e := range history {
					printer <- e
				}
				return // Gorutyna podróżnika kończy działanie
			} else { // reservationResult == 0 (zajęte)
				time.Sleep(1 * time.Millisecond)
				continue // spróbuj ponownie
			}
		}

		if !moved {
			deadlock = true
			if symbol >= 'A' && symbol <= 'Z' {
				symbol = rune(symbol - 'A' + 'a')
			}
			recordEvent(pos, symbol)
			break
		}
	}

	if !deadlock {
		//buforowany kanal bo inaczej nie dziala :(
		release := make(chan int, 1)
		board[pos.x][pos.y] <- BoardCommand{action: "release", replyChan: release, travelerId: id}
		<-release
	}

	for _, e := range history {
		printer <- e
	}
}

type Squatter struct {
	id      int
	symbol  rune
	current Point
	cmdChan chan string
}

func simulateSquatter(s Squatter, printer chan<- string) {
	var history []string
	recordEvent := func(pos Point, sym rune) {
		e := fmt.Sprintf("%d %d %d %d %c", time.Now().UnixNano(), s.id, pos.x, pos.y, sym)
		history = append(history, e)
	}

	directions := []struct{ dx, dy int }{
		{-1, 0}, {1, 0}, {0, -1}, {0, 1},
	}

	s.cmdChan = make(chan string, 1)

	// próbujemy zarezerwować losowe wolne pole
	for {
		x, y := rand.Intn(Board_Width), rand.Intn(Board_Height)
		r := make(chan int, 1)
		board[x][y] <- BoardCommand{action: "squatterReserve", replyChan: r, travelerId: s.id}
		replyVal := <-r // ODCZYTAJ RAZ

		if replyVal == 1 { // Sukces
			pos := Point{x, y}
			s.current = pos
			recordEvent(pos, s.symbol)
			board[pos.x][pos.y] <- BoardCommand{action: "setSquatterChannel", squatChan: s.cmdChan}
			break
		} else if replyVal == 10 { // Pułapka przy próbie startu
			// Squatter po prostu próbuje innego miejsca, nie "ginie" na starcie.
			continue
		} else { // replyVal == 0 (zajęte lub inny błąd)
			time.Sleep(1 * time.Millisecond)
			continue
		}
	}
	lifeTime := 0

	running := true
	for running {
		select {
		case cmd := <-s.cmdChan:
			if cmd == "kick" {
				lifeTime += 10 // Rozważ, czy ta logika jest potrzebna
				// movedSuccessfully := false // Flaga do śledzenia, czy ruch się udał
				for _, d := range directions {
					newPos := move(s.current, d.dx, d.dy)
					r := make(chan int, 1)
					board[newPos.x][newPos.y] <- BoardCommand{action: "squatterReserve", replyChan: r, travelerId: s.id}
					replyVal := <-r // ODCZYTAJ RAZ

					if replyVal == 1 { // Udało się zająć nowe pole
						// Zwalniamy poprzednie pole
						r2 := make(chan int, 1)
						board[s.current.x][s.current.y] <- BoardCommand{action: "release", replyChan: r2, travelerId: s.id}
						<-r2 // Poczekaj na potwierdzenie zwolnienia

						s.current = newPos // Zaktualizuj pozycję squattera
						// Ustaw kanał squattera dla nowego pola
						board[s.current.x][s.current.y] <- BoardCommand{action: "setSquatterChannel", squatChan: s.cmdChan}
						recordEvent(newPos, s.symbol)
						// movedSuccessfully = true
						break // Wyjdź z pętli kierunków
					} else if replyVal == 10 { // Wpadł na pułapkę próbując się przenieść
						// Zwolnij pole, które squatter zajmował PRZED próbą ruchu
						r2 := make(chan int, 1)
						board[s.current.x][s.current.y] <- BoardCommand{action: "release", replyChan: r2, travelerId: s.id}
						<-r2                               // Poczekaj na potwierdzenie zwolnienia
						recordEvent(s.current, '.')        // Oznacz stare pole jako opuszczone (.)
						recordEvent(newPos, '*')           // Zapisz próbę wejścia na pułapkę '*'
						time.Sleep(200 * time.Millisecond) // chwilowe zajęcie pola
						// Zwolnienie pola
						board[newPos.x][newPos.y] <- BoardCommand{action: "release", replyChan: r2, travelerId: s.id}
						<-r2 // odbierz odpowiedź
						// Ślad śmierci – pułapka pojawia się ponownie
						recordEvent(newPos, '#') // Pułapka pojawia się ponownie
						// Wypisz ślad pułapki
						// Wypisz historię i zakończ wątek squattera
						for _, e := range history {
							printer <- e
						}
						return // Koniec wątku squattera
					} else { // replyVal == 0 (nowe pole zajęte lub inny problem)
						// Spróbuj następnego kierunku
						continue
					}
				}
				// if !movedSuccessfully {
				// Squatter został eksmitowany, ale nie mógł znaleźć nowego miejsca.
				// Pozostaje na s.current. Podróżnik, który go eksmitował, otrzymał 0 (zajęte)
				// i spróbuje ponownie. To zachowanie wydaje się w porządku.
				// }
			}
		default:
			time.Sleep(10 * time.Millisecond)
			lifeTime += 10
		}

		if lifeTime > SquatterLifeTime {
			running = false
			// zwalniamy pole
			r := make(chan int, 1)
			board[s.current.x][s.current.y] <- BoardCommand{action: "release", replyChan: r, travelerId: s.id}
			<-r
			recordEvent(s.current, '.')
		}
	}

	// wypisanie historii squattera
	for _, e := range history {
		printer <- e
	}
}

///////////////////////
///////////////////////

func main() {
	rand.Seed(time.Now().UnixNano())

	generateTraps()

	printer := make(chan string, 100)
	var wg sync.WaitGroup

	go func() {
		for event := range printer {
			fmt.Println(event)
		}
	}()

	// Inicjalizacja planszy jako serwerów pól
	for i := 0; i < Board_Width; i++ {
		for j := 0; j < Board_Height; j++ {
			//buforowany kanal bo inaczej nie dziala :(
			board[i][j] = make(chan BoardCommand, 1)
			go startFieldServer(board[i][j], printer, i, j)
		}
	}

	fmt.Printf("-1 %d %d %d\n", Nr_Of_Travelers+Max_Squatters+Num_Traps, Board_Width, Board_Height)

	wg.Add(Nr_Of_Travelers)
	for id := 1; id <= Nr_Of_Travelers; id++ {
		go simulateTraveler(id, printer, &wg)
	}
	// uruchom squattery jako osobne wątki
	for i := 0; i < Max_Squatters; i++ {
		sym := rune('0' + i)
		wg.Add(1)
		go func(id int, sym rune) {
			defer wg.Done()
			s := Squatter{id, sym, Point{-1, -1}, make(chan string, 1)}
			simulateSquatter(s, printer)
		}(1000+i, sym)
		// przerwa między uruchomieniem kolejnego squattera
		time.Sleep(time.Millisecond * time.Duration(SquatterIntervalMs))
	}

	wg.Wait()
	close(printer)
}
