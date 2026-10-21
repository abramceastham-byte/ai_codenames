package main

import (
	"strings"
	"testing"

	"github.com/bcspragu/Codenames/arena"
	"github.com/bcspragu/Codenames/botproto"
)

func match(red, blue string, winner botproto.Team, forfeits ...botproto.Team) *arena.Match {
	m := &arena.Match{Result: &botproto.Result{Winner: winner}}
	m.Info.Red.Name, m.Info.Blue.Name = red, blue
	for _, t := range forfeits {
		m.Result.Events = append(m.Result.Events, botproto.Event{Team: t})
	}
	return m
}

func TestStandings(t *testing.T) {
	s := newStandings([]string{"a", "b", "c"})
	s.add(match("a", "b", botproto.TeamBlue))                    // b beats a
	s.add(match("b", "a", botproto.TeamRed))                     // b beats a
	s.add(match("a", "c", "", botproto.TeamRed))                 // draw, a forfeits once
	s.add(match("c", "a", botproto.TeamBlue))                    // a beats c
	s.add(match("b", "c", botproto.TeamBlue, botproto.TeamBlue)) // c beats b, c forfeits once
	s.add(match("c", "b", ""))                                   // draw

	var out strings.Builder
	s.print(&out)
	got := strings.Fields(out.String())
	want := strings.Fields(`Rank Team Played Wins Losses Draws Forfeits
1 b 4 2 1 1 0
2 c 4 1 1 2 1
3 a 4 1 2 1 1`)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("standings:\n%s\nwant:\n%s", out.String(), strings.Join(want, " "))
	}

	// Teams tied on wins, draws, and forfeits share a rank.
	tied := newStandings([]string{"x", "y"})
	tied.add(match("x", "y", botproto.TeamRed))
	tied.add(match("y", "x", botproto.TeamRed))
	out.Reset()
	tied.print(&out)
	got = strings.Fields(out.String())
	want = strings.Fields(`Rank Team Played Wins Losses Draws Forfeits
1 x 2 1 1 0 0
1 y 2 1 1 0 0`)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("standings:\n%s\nwant:\n%s", out.String(), strings.Join(want, " "))
	}
}
