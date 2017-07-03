package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import "labrpc"
import "fmt"
import "time"
import "math/rand"

import "bytes"
import "encoding/gob"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

// simple key-value pair
type Pair struct {
	Command interface{}
	Term    int
}

// appendEntries rpc parameter

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	currentTerm   int
	currentLeader int
	votedFor      int
	LogEntries    []Pair

	CommitIndex   int
	LastCommitted int

	// if this peer come into power,the following structure will be constructed
	// otherwise,it will keeps nil value
	nextIndex  []int
	matchIndex []int

	// messaging channel
	timeoutChan  chan int
	sigrstChan   chan int
	toworkerChan chan int // receive messeage on receiving valid AppendEntries RPC

	// running state indicator
	RUNNING_STATE bool
	TIMER_STATE   bool

	// debugs
	DEBUG_SWITCH bool
}

func (rf *Raft) debugPrint(outer func()) {
	if rf.DEBUG_SWITCH {
		outer()
	}
}

func (rf *Raft) Timer(durationMs int) {
	duration := durationMs
	ticks := duration
	for rf.RUNNING_STATE {
		select {
		case newdur := <-rf.sigrstChan:
			// signal reset timer
			if newdur >= 0 {
				duration = newdur
			}
			ticks = duration
		case <-time.After(time.Millisecond):
			// tick event.
			ticks--
		}
		if ticks <= 0 { // avoid the situation that accidentially set the dur to neg numbers
			// timeout.
			// on timing out, the timer will automatically pause
			// and reset the tick event counter
			rf.debugPrint(func() { fmt.Printf("Server %d Timed out.\n", rf.me) })
			// in case signal reset may be sent during this process, check reset channel
			rf.timeoutChan <- duration
			newDuration := <-rf.sigrstChan
			if newDuration >= 0 {
				duration = newDuration
			}
			ticks = duration
		}
	}
}

// set the timer
func (rf *Raft) TimerSet(durationMs int) {
	select {
	case <-rf.timeoutChan:
	default:
	}
	select {
	case <-rf.sigrstChan:
	default:
	}
	rf.sigrstChan <- durationMs
}

// reset the timer
func (rf *Raft) TimerReset() {
	rand.Seed(time.Now().Unix() + int64(rf.me))
	rf.TimerSet(rand.Intn(500) + 200)
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	// Your code here.
	return rf.currentTerm, rf.matchIndex != nil
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.LogEntries)
	e.Encode(rf.LastCommitted)
	e.Encode(rf.CommitIndex)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.LogEntries)
	d.Decode(&rf.LastCommitted)
	d.Decode(&rf.CommitIndex)
}

//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
	// Your data here.
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

//
// example RequestVote RPC reply structure.
//
type RequestVoteReply struct {
	// Your data here.
	Term        int
	VoteGranted bool
}

// appendEntries rpc args
type AppendEntriesArgs struct {
	Term              int
	LeaderId          int // i am leader!
	PrevLogIndex      int
	PrevLogTerm       int
	Entries           []Pair
	LeaderCommitIndex int
}

type AppendEntriesReply struct {
	CurrTerm    int
	Success     bool
	LastApplied int
}

// appendEntries RPC handler
func (rf *Raft) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.debugPrint(func() { fmt.Printf("(%d)Sever %d have a AE rpc %v\n", rf.currentTerm, rf.me, args) })
	reply.CurrTerm = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.Success = false
		return
	}
	rf.TimerReset()
	// ok in this case (lab 2A) I wont care what args contains,
	// I will just take it as heartbeat package
	// TODO : implement 2B 2C
	rf.currentLeader = args.LeaderId
	if args.Term >= rf.currentTerm {
		// set current term
		rf.currentTerm = args.Term
		// send towork signal
		// clear first
		select {
		case <-rf.toworkerChan:
		default:
		}
		go func() {
			rf.toworkerChan <- args.LeaderId
		}()
		// update logEntries
		rf.debugPrint(func() { fmt.Printf("(%d)Server %d former logs are %v\n", rf.currentTerm, rf.me, rf.LogEntries) })
		if len(rf.LogEntries) <= args.PrevLogIndex || rf.LogEntries[args.PrevLogIndex].Term != args.PrevLogTerm {
			rf.debugPrint(func() {
				if len(rf.LogEntries) <= args.PrevLogIndex {
					fmt.Printf("(%d)Server %d cmp length failed with %d/%d\n", rf.currentTerm, rf.me, args.PrevLogIndex, len(rf.LogEntries))
				} else {
					fmt.Printf("(%d)Server %d cmp term failed with %d<>%d\n", rf.currentTerm, rf.me, rf.LogEntries[args.PrevLogIndex].Term, args.PrevLogTerm)
				}
			})
			reply.LastApplied = rf.LastCommitted
			reply.Success = false
		} else {
			reply.Success = true
			// shrink to fit
			if len(args.Entries) > 1 {
				rf.LogEntries = rf.LogEntries[0 : args.PrevLogIndex+1]
				rf.debugPrint(func() { fmt.Printf("(%d)Server %d received args %v\n", rf.currentTerm, rf.me, args) })
				for i := 1; i < len(args.Entries); i++ {
					rf.LogEntries = append(rf.LogEntries, args.Entries[i])
				}
				rf.debugPrint(func() { fmt.Printf("(%d)Server %d updated result is %v\n", rf.currentTerm, rf.me, rf.LogEntries) })
			}
			if args.LeaderCommitIndex > rf.CommitIndex {
				rf.CommitIndex = args.LeaderCommitIndex
			}
		}
	}
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
	rf.TimerReset()
	reply.Term = args.Term
	rf.debugPrint(func() { fmt.Printf("(%d)Server %d received a vote %v.\n", rf.currentTerm, rf.me, args) })
	// check currency and completion
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}
	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.votedFor = -1
	}
	if args.LastLogTerm >= rf.LogEntries[rf.LastCommitted].Term && args.LastLogIndex >= rf.LastCommitted {
		// you are the big brother ... or I have already voted for somebody!
		if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
			// you are the big brother
			rf.votedFor = args.CandidateId
			rf.debugPrint(func() { fmt.Printf("(%d)Server %d gives %d a vote.\n", rf.currentTerm, rf.me, args.CandidateId) })
			reply.VoteGranted = true
		} else {
			// no way
			rf.debugPrint(func() {
				fmt.Printf("(%d)Server %d refused to vote because it has voted for %d.\n", rf.currentTerm, rf.me, rf.votedFor)
			})
			reply.VoteGranted = false
		}
	} else {
		// I am the big brother
		rf.debugPrint(func() {
			fmt.Printf("(%d)Server %d refuse to vote because cmp fail with (%d,%d)&(%d,%d)\n", rf.currentTerm, rf.me, args.LastLogTerm, rf.LogEntries[rf.LastCommitted].Term, args.LastLogIndex, rf.LastCommitted)
		})
		reply.VoteGranted = false
	}
}

func (rf *Raft) ApplyEntries(entry Pair) {
	// TODO : this is not used in case lab2 doesn't have a real statemaching running
	// TODO : Implement this in following labs
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	isSuccChan := make(chan bool)
	go func() {
		ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
		isSuccChan <- ok
	}()
	select {
	case rev := <-isSuccChan:
		return rev
	case <-time.After(500 * time.Millisecond):
		return false
	}
}

// send an AppendEntries RPC to a server
func (rf *Raft) sendAppendEntries(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	isSuccChan := make(chan bool)
	go func() {
		ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
		isSuccChan <- ok
	}()
	select {
	case rev := <-isSuccChan:
		return rev
	case <-time.After(500 * time.Millisecond):
		return false
	}
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	if rf.nextIndex == nil {
		return len(rf.LogEntries) - 1, rf.currentTerm, false
	}
	// That's the leader.
	// Attempt to add this entry to stm
	rf.debugPrint(func() { fmt.Printf("(%d)Server %d received client request %v\n", rf.currentTerm, rf.me, command) })
	rf.mu.Lock()
	rf.LogEntries = append(rf.LogEntries, Pair{command, rf.currentTerm})
	afterlen := len(rf.LogEntries) - 1
	rf.mu.Unlock()
	rf.debugPrint(func() { fmt.Printf("(%d)Server %d result entries are %v\n", rf.currentTerm, rf.me, rf.LogEntries) })
	return afterlen, rf.currentTerm, true
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
	rf.debugPrint(func() { fmt.Printf("Server %d Closed.\n", rf.me) })
	rf.DEBUG_SWITCH = false // this server is dead. not needed.
	rf.RUNNING_STATE = false
}

// this process keeps running until it was killed.
func (rf *Raft) StateMachine() {
	for rf.RUNNING_STATE {
		// begin as worker:
		// wait until timer timeout
	FollowerProcess:
		rf.debugPrint(func() { fmt.Printf("(%d)Server %d become follower.\n", rf.currentTerm, rf.me) })
		rf.TimerReset()
		rf.nextIndex = nil
		rf.matchIndex = nil
		rf.votedFor = -1
		select {
		case <-rf.timeoutChan:
		}
		// then I am a candidate!
		//ElectionProcess:
		select {
		case <-rf.toworkerChan:
		default:
		}
		rf.debugPrint(func() { fmt.Printf("(%d)Server %d become candidate.\n", rf.currentTerm, rf.me) })
		rf.currentTerm++
		rf.votedFor = rf.me
		gatheredVotes := 1
		rf.TimerReset()
		for i := 0; i < len(rf.peers); i++ {
			if i != rf.me {
				// no one will asks himself
				go func(i int, gatheredVotes *int) {
					invokeTerm := rf.currentTerm
					reply := new(RequestVoteReply)
					args := RequestVoteArgs{rf.currentTerm, rf.me, rf.CommitIndex, 0}
					args.LastLogTerm = rf.LogEntries[len(rf.LogEntries)-1].Term
					rf.debugPrint(func() { fmt.Printf("(%d)Server %d asks %d for a vote %v\n", rf.currentTerm, rf.me, i, args) })
					ok := rf.sendRequestVote(i, args, reply)
					if ok && reply.VoteGranted && reply.Term == invokeTerm {
						rf.mu.Lock()
						*gatheredVotes++
						rf.mu.Unlock()
						rf.debugPrint(func() { fmt.Printf("(%d)Server %d gets %d votes.\n", rf.currentTerm, rf.me, *gatheredVotes) })
					} else {
						rf.debugPrint(func() { fmt.Printf("(%d)Vote Gathering %d<-%d Failed: %v %v\n", rf.currentTerm, rf.me, i, ok, reply) })
						if reply.Term > rf.currentTerm {
							rf.currentTerm = reply.Term
							go func() {
								select {
								case <-rf.toworkerChan:
								default:
								}
								rf.toworkerChan <- 0
							}()
						}
					}
				}(i, &gatheredVotes)
			}
		}
		// all RequestVote RPCs are sent
		// patiently waiting for them to return ... or not
		for {
			select {
			case <-rf.timeoutChan:
				// next term
				goto FollowerProcess
			case <-rf.toworkerChan:
				// there's a leader that comes to power
				// go back to followers
				goto FollowerProcess
			default:
			}
			if !rf.RUNNING_STATE {
				return
			}
			if gatheredVotes > len(rf.peers)/2 {
				break
			}
		}
		// I come to power!
		// note in this part (lab2A) i will not care what the leader have sent
		// the leader will just sent empty stuffs(hb pack)
		// TODO : lab2B lab2C

		rf.debugPrint(func() { fmt.Printf("(%d)Server %d become leader.\n", rf.currentTerm, rf.me) })
		successive := make([]bool, len(rf.peers))
		// first set all to success..
		for i := 0; i < len(successive); i++ {
			successive[i] = true
		}
		rf.nextIndex = make([]int, len(rf.peers))
		// init rf.nextIndex
		for i := 0; i < len(rf.peers); i++ {
			rf.nextIndex[i] = rf.CommitIndex
		}
		rf.matchIndex = make([]int, len(rf.peers))
		// init rf.matchIndex
		for i := 0; i < len(rf.peers); i++ {
			rf.matchIndex[i] = 0
		}
		for i := 0; i < len(rf.peers); i++ {
			// launch goroutines to send rpc calls
			if i != rf.me {
				go func(i int) {
					for rf.nextIndex != nil && rf.RUNNING_STATE {
						// well i wont mind about what the leaders will do about it at this part (lab2A)
						// but shall implement on later parts
						// TODO : lab2B lab2C
						reply := new(AppendEntriesReply)
						args := AppendEntriesArgs{rf.currentTerm, rf.me, 0, 0, make([]Pair, 0), rf.CommitIndex}
						// create values & slices
						if rf.nextIndex != nil {
							args.PrevLogIndex = rf.nextIndex[i]
							args.PrevLogTerm = rf.LogEntries[rf.nextIndex[i]].Term
							args.Entries = rf.LogEntries[rf.nextIndex[i]:len(rf.LogEntries)]
						}
						// send AppendEntries RPC
						rf.debugPrint(func() { fmt.Printf("(%d)Server %d have entries %v\n", rf.currentTerm, rf.me, rf.LogEntries) })
						rf.debugPrint(func() { fmt.Printf("(%d)Server %d send %d args %v\n", rf.currentTerm, rf.me, i, args) })
						ok := rf.sendAppendEntries(i, args, reply)
						successive[i] = ok
						if !ok {
							rf.debugPrint(func() { fmt.Printf("(%d)Server %d network failed.\n", rf.currentTerm, rf.me) })
						} else {
							// so the AE RPC is success and have its return value.
							// lets check it.
							rf.debugPrint(func() { fmt.Printf("(%d)Server %d network is working.\n", rf.currentTerm, rf.me) })
							if reply.Success {
								if rf.nextIndex != nil {
									rf.nextIndex[i] = len(rf.LogEntries) - 1
									rf.matchIndex[i] = len(rf.LogEntries) - 1
								}
							} else {
								if rf.nextIndex != nil {
									rf.nextIndex[i] = reply.LastApplied
								}
							}
						}
						// well this part will not care about what that stuff returns.
						time.Sleep(50 * time.Millisecond)
					}
				}(i)
			}
		}
		for rf.RUNNING_STATE {
			towk := -1
			select {
			case towk = <-rf.toworkerChan:
			default:
			}
			if towk != -1 {
				break
			}
			failsum := 0
			for x := 0; x < len(successive); x++ {
				if !successive[x] {
					failsum++
				}
			}
			if failsum >= len(successive)/2 {
				rf.debugPrint(func() {
					fmt.Printf("(%d)Server %d reached fail limit(%d/%d).\n", rf.currentTerm, rf.me, failsum, len(successive)/2)
				})
				break
			}
		}
	}
}

// this process keeps applying new entries
func (rf *Raft) Applier(applyCh chan ApplyMsg) {
	for rf.RUNNING_STATE {
		time.Sleep(50 * time.Millisecond)
		if rf.CommitIndex > rf.LastCommitted && rf.LastCommitted+1 < len(rf.LogEntries) {
			rf.debugPrint(func() {
				fmt.Printf("(%d)Server %d attempt to commit %d/%d with cond %d,%d\n", rf.currentTerm, rf.me, rf.LastCommitted+1, len(rf.LogEntries), rf.CommitIndex, rf.LastCommitted)
			})
			rf.ApplyEntries(rf.LogEntries[rf.LastCommitted+1])
			rf.LastCommitted++
			applyCh <- ApplyMsg{rf.LastCommitted, rf.LogEntries[rf.LastCommitted].Command, false, nil}
			rf.persist()
			rf.debugPrint(func() {
				fmt.Printf("(%d)Server %d commited change %d,%v\n", rf.currentTerm, rf.me, rf.LastCommitted, rf.LogEntries[rf.LastCommitted])
			})

		} else if rf.nextIndex != nil && rf.LastCommitted+1 < len(rf.LogEntries) {
			// check agreement with cluster
			rf.debugPrint(func() { fmt.Printf("(%d)Server %d have matchIndex record %v\n", rf.currentTerm, rf.me, rf.matchIndex) })
			sum := 1
			for i := 0; i < len(rf.peers); i++ {
				if i != rf.me {
					if rf.matchIndex != nil && rf.matchIndex[i] >= rf.LastCommitted+1 {
						sum++
					}
				}
			}
			rf.debugPrint(func() {
				fmt.Printf("(%d)Server %d attempt to commit %d/%d with %d agreement\n", rf.currentTerm, rf.me, rf.LastCommitted+1, len(rf.LogEntries), sum)
			})
			if sum > len(rf.peers)/2 {
				// most cluster agree with leader
				// start commit
				rf.ApplyEntries(rf.LogEntries[rf.LastCommitted+1])
				rf.LastCommitted++
				if rf.CommitIndex < rf.LastCommitted {
					rf.CommitIndex = rf.LastCommitted
				}
				applyCh <- ApplyMsg{rf.LastCommitted, rf.LogEntries[rf.LastCommitted].Command, false, nil}
				rf.persist()
				rf.debugPrint(func() {
					fmt.Printf("(%d)Server %d commited change %d,%v\n", rf.currentTerm, rf.me, rf.LastCommitted, rf.LogEntries[rf.LastCommitted])
				})
			}
		}
	}
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rand.Seed(time.Now().Unix() + int64(me))
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here.

	rf.DEBUG_SWITCH = false // open debug output by default
	rf.RUNNING_STATE = true // ensure the stm loop keeps going
	rf.TIMER_STATE = true   // ensure continuous timer

	// setting up channels

	rf.timeoutChan = make(chan int)
	rf.sigrstChan = make(chan int)

	// first, recover currTerm,votedFor: these values are independent with the persisted values,
	rf.currentTerm = 0
	rf.votedFor = -1
	rf.currentLeader = -1

	// initialize from state persisted before a crash
	rf.LogEntries = make([]Pair, 0)
	rf.LogEntries = append(rf.LogEntries, Pair{nil, 0})
	rf.CommitIndex = 0
	rf.LastCommitted = 0
	rf.readPersist(persister.ReadRaftState())

	// finally, run the state machine and the timer
	go rf.Timer(rand.Intn(500) + 200)
	go rf.StateMachine()
	go rf.Applier(applyCh)

	return rf
}
