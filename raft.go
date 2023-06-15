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

import (
	"bytes"
	"cs350/labgob"
	"cs350/labrpc"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// import "bytes"
// import "cs350/labgob"

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	CurrentTerm   int
	VotedFor      int
	CommitIndex   int
	LastApplied   int
	NextIndex     []int //next index of my log to send to each other server, only used when leader, init to leader last log index +1
	MatchIndex    []int //index of highest log entry known to be rep'd on each server, only used when leader, init to 0
	Log           []LogEntry
	LastHeartbeat time.Time //update each time you get an append entries or cast a vote, compare with current time
	VotesReceived int
	LastAppended  int //variable for index last appended to the log
	IsLeader      bool
	applyChannel  chan ApplyMsg
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	var term int
	var isleader bool
	// Your code here (2A).
	term = rf.CurrentTerm
	isleader = rf.IsLeader

	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.CurrentTerm)
	e.Encode(rf.VotedFor)
	e.Encode(rf.Log)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var CurrentTerm int
	var VotedFor int
	var Log []LogEntry
	if d.Decode(&CurrentTerm) != nil ||
		d.Decode(&VotedFor) != nil || d.Decode(&Log) != nil {
		println("*****error reading state*****")
	} else {
		rf.CurrentTerm = CurrentTerm
		rf.VotedFor = VotedFor
		rf.Log = Log
	}
}

// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshot []byte) bool {

	// Your code here (2D).

	return true
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

type AppendEntriesArgs struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term          int
	Success       bool
	RejectedTerm  int //for conflict optimization
	RejectedIndex int //for conflict optimization
}

type LogEntry struct {
	Term    int
	Index   int
	Command interface{}
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.CurrentTerm { //if candidate term less than our term, reject
		//println(rf.me, "did not vote for ", args.CandidateID, "because their term is less")
		reply.Term = rf.CurrentTerm
		reply.VoteGranted = false

	} else if args.Term > rf.CurrentTerm { //step down
		//println(rf.me, "has updated term and is a follower since there is a candidate with a higher term. Checking log")
		rf.CurrentTerm = args.Term
		rf.IsLeader = false
		rf.VotesReceived = 0
		rf.VotedFor = -1
		lastLogEntry := rf.Log[len(rf.Log)-1]
		go rf.persist()
		//println(lastLogEntry.Term, args.LastLogTerm)
		if lastLogEntry.Term > args.LastLogTerm { //reject if our log entry has a higher term than theirs
			//println(rf.me, "did not vote for ", args.CandidateID, "due to our log being more recent")
			reply.Term = rf.CurrentTerm
			reply.VoteGranted = false
		} else if lastLogEntry.Term == args.LastLogTerm {
			if lastLogEntry.Index > args.LastLogIndex { //reject if our log entries have same term but ours is longer length
				//println(rf.me, "did not vote for ", args.CandidateID, "due to our log being more recent")
				reply.Term = rf.CurrentTerm
				reply.VoteGranted = false
			} else { //accept otherwise
				//println(rf.me, "voted for ", args.CandidateID)
				reply.Term = args.Term
				reply.VoteGranted = true
				rf.VotedFor = args.CandidateID
				rf.CurrentTerm = args.Term
				rf.IsLeader = false
				rf.LastHeartbeat = time.Now()
				go rf.persist()
			}
		} else { //accept
			//println(rf.me, "voted for ", args.CandidateID)
			reply.Term = args.Term
			reply.VoteGranted = true
			rf.VotedFor = args.CandidateID
			rf.CurrentTerm = args.Term
			rf.IsLeader = false
			rf.LastHeartbeat = time.Now()
			go rf.persist()
		}

	} else { //check VotedFor == -1 || VotedFor == args.CandidateID, then check log, if log is good, grant vote
		if rf.VotedFor == -1 || rf.VotedFor == args.CandidateID { //check log
			lastLogEntry := rf.Log[len(rf.Log)-1]
			if lastLogEntry.Term > args.LastLogTerm { //reject if our log entry has a higher term than theirs
				//println(rf.me, "did not vote for ", args.CandidateID, "due to our log being more recent")
				reply.Term = rf.CurrentTerm
				reply.VoteGranted = false
			} else if lastLogEntry.Term == args.LastLogTerm {
				if lastLogEntry.Index > args.LastLogIndex { //reject if our log entries have same term but ours is longer length
					//println(rf.me, "did not vote for ", args.CandidateID, "due to our log being more recent")
					reply.Term = rf.CurrentTerm
					reply.VoteGranted = false
				} else { //accept otherwise
					//println(rf.me, "voted for ", args.CandidateID)
					reply.Term = args.Term
					reply.VoteGranted = true
					rf.VotedFor = args.CandidateID
					rf.CurrentTerm = args.Term
					rf.IsLeader = false
					rf.LastHeartbeat = time.Now()
					go rf.persist()
				}
			}
			// } else { //accept otherwise
			// 	println(rf.me, "voted for ", args.CandidateID)
			// 	reply.Term = args.Term
			// 	reply.VoteGranted = true
			// 	rf.VotedFor = args.CandidateID
			// 	rf.CurrentTerm = args.Term
			// 	rf.IsLeader = false
			// 	rf.LastHeartbeat = time.Now()
		} else { //do not grant vote, already voted in this election
			//println(rf.me, "did not vote for ", args.CandidateID, "because already voted this election")
			reply.Term = rf.CurrentTerm
			reply.VoteGranted = false
		}

	}
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.CurrentTerm { //if less than current term (condition 1)
		//println(rf.me, "rejected AppendEntries from", args.LeaderID, "term:", rf.CurrentTerm)
		//println(args.Term, rf.CurrentTerm)
		reply.Term = rf.CurrentTerm
		reply.Success = false
	} else { //else do more checks
		if args.Term > rf.CurrentTerm {
			rf.CurrentTerm = args.Term
			rf.IsLeader = false
			rf.VotedFor = -1
			rf.VotesReceived = 0
			go rf.persist()
		}
		if len(rf.Log) > args.PrevLogIndex {
			log_check := rf.Log[args.PrevLogIndex]
			if log_check.Term != args.PrevLogTerm { //reply false if terms don't match (condition 2)
				//println(rf.me, "rejected AppendEntries from", args.LeaderID, "because logs don't match. Term:", rf.CurrentTerm)
				reply.Term = rf.CurrentTerm
				reply.Success = false
				reply.RejectedTerm = log_check.Term
				for i := args.PrevLogIndex; i > 0; i-- {
					if rf.Log[i].Term < log_check.Term {
						reply.RejectedIndex = rf.Log[i+1].Index
						break
					}
				}
			} else { //accept
				go rf.check_apply(args.LeaderCommit)
				if len(args.Entries) > 0 {
					rf.Log = rf.Log[:(args.PrevLogIndex + 1)]
					rf.Log = append(rf.Log, args.Entries...)
					go rf.persist()
					//fmt.Println(rf.me, "accepted AppendEntries from", args.LeaderID)
				} else {
					//println(rf.me, "accepted heartbeat from", args.LeaderID)
				}

				reply.Term = args.Term
				reply.Success = true
				rf.LastHeartbeat = time.Now()
				go rf.check_apply(args.LeaderCommit) //condition 5
			}
		} else { //we don't have this entry, must catch up
			//println(rf.me, "rejected AppendEntries from", args.LeaderID, "because logs don't match. Term:", rf.CurrentTerm)
			reply.Term = rf.CurrentTerm
			reply.Success = false
			reply.RejectedTerm = rf.Log[len(rf.Log)-1].Term
			reply.RejectedIndex = rf.Log[len(rf.Log)-1].Index
		}

	}
}

func (rf *Raft) check_apply(leaderCommit int) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if leaderCommit > rf.CommitIndex {
		var newIndex int
		if leaderCommit > len(rf.Log)-1 {
			newIndex = len(rf.Log) - 1
		} else {
			newIndex = leaderCommit
		}
		for rf.CommitIndex < newIndex {
			var apply ApplyMsg
			logToApply := rf.Log[rf.CommitIndex+1]
			apply.Command = logToApply.Command
			apply.CommandIndex = logToApply.Index
			apply.CommandValid = true
			rf.CommitIndex += 1
			rf.LastApplied += 1
			rf.applyChannel <- apply
			//fmt.Println(rf.me, "applied entry to match leader")
		}
	}
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	if reply.VoteGranted { //if we get a vote, increase VotesReceived
		rf.mu.Lock()
		rf.VotesReceived += 1
		//println(rf.me, "vote received from", server)
		rf.mu.Unlock()
	}
	return ok
} //don't have to change anything here

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	alive := !rf.killed()
	leader := rf.IsLeader
	rf.mu.Unlock()
	if leader && alive {
		rf.peers[server].Call("Raft.AppendEntries", args, reply)
		ret_term, ret_succ := reply.Term, reply.Success
		rf.mu.Lock()
		this_term := rf.CurrentTerm
		rf.mu.Unlock()
		if !ret_succ {
			if this_term < ret_term { //if we get a response from a newer term, we are no longer the leader
				rf.mu.Lock()
				rf.CurrentTerm = ret_term
				rf.IsLeader = false
				rf.VotesReceived = 0
				//println(server, "rejected heartbeat term:", this_term, rf.me, "no longer leader")
				rf.LastHeartbeat = time.Now()
				go rf.persist()
				rf.mu.Unlock()

			} else if this_term == ret_term { //if we get rejected during the current term, the receiver may not have been up to date
				//decrement and retry
				rf.mu.Lock()
				rf.NextIndex[server] = 1
				var newReply AppendEntriesReply
				var newArgs AppendEntriesArgs
				newArgs.Term = rf.CurrentTerm
				newArgs.LeaderID = rf.me
				newArgs.LeaderCommit = rf.CommitIndex
				newArgs.PrevLogIndex = 0
				newArgs.Entries = rf.Log[1:]
				newArgs.PrevLogTerm = 0
				rf.mu.Unlock()
				go rf.sendAppendEntries(server, &newArgs, &newReply)
			}
		} else {
			if len(args.Entries) > 0 {
				lastEntry := args.Entries[len(args.Entries)-1]
				rf.mu.Lock()
				rf.NextIndex[server] = lastEntry.Index + 1
				rf.MatchIndex[server] = lastEntry.Index
				//println(rf.NextIndex[server], rf.MatchIndex[server])
				rf.mu.Unlock()
				go rf.commit(args.PrevLogIndex, len(args.Entries))
			}
		}
	}
}

func (rf *Raft) commit(lastIndex int, numEntries int) {
	N := lastIndex + numEntries
	//println(N)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if N > len(rf.Log) {
		N = len(rf.Log) - 1
	}
	if rf.Log[N].Term == rf.CurrentTerm && rf.CommitIndex < N {
		numReached := 1
		for i := 0; i < len(rf.peers); i++ {
			if i != rf.me {
				if rf.MatchIndex[i] >= N {
					numReached += 1
				}
			}
		}
		if numReached >= ((len(rf.peers) / 2) + 1) {
			for rf.CommitIndex < N {
				var apply ApplyMsg
				apply.Command = rf.Log[rf.CommitIndex+1].Command
				apply.CommandValid = true
				apply.CommandIndex = rf.Log[rf.CommitIndex+1].Index
				rf.applyChannel <- apply
				//fmt.Println("leader", rf.me, "applied command at index", apply.CommandIndex)
				rf.CommitIndex += 1
				rf.LastApplied += 1
			}

			//println(rf.Log[0].Index, rf.Log[0].Term, rf.Log[1].Index, rf.Log[1].Term)
		}
	}
	// a single N greater than commit index where the term = current term and has reached maj of servers
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	alive := !rf.killed()
	if rf.IsLeader && alive {
		//go through each peer and send append entries w/ go routines

		var entry LogEntry
		entry.Command = command
		entry.Term = rf.CurrentTerm
		entry.Index = len(rf.Log)

		var args AppendEntriesArgs
		args.Entries = make([]LogEntry, 1)

		args.Entries[0] = entry

		args.LeaderCommit = rf.CommitIndex
		args.LeaderID = rf.me
		args.Term = rf.CurrentTerm
		args.PrevLogIndex = len(rf.Log) - 1
		args.PrevLogTerm = rf.Log[args.PrevLogIndex].Term
		//println(args.PrevLogIndex, args.PrevLogTerm, len(rf.Log))
		index = len(rf.Log)

		rf.Log = append(rf.Log, entry)
		go rf.persist()
		//println("sending new log entry, index:", entry.Index, "term:", entry.Term)

		for i := 0; i < len(rf.peers); i++ {
			if i != rf.me {
				var reply AppendEntriesReply
				go rf.sendAppendEntries(i, &args, &reply)
			}
		}
		term = rf.CurrentTerm
		//println(index, term)
	} else {
		isLeader = false
	}

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
} //not required function, just to help debug (will be useful for part C), kills the raft process to view what's going on

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {
	rf.mu.Lock()
	me := rf.me
	num_peers := len(rf.peers)
	rf.mu.Unlock()
	var isLeader bool
	for !rf.killed() {
		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().

		var time_to_wait int = rand.Intn(100) + 500
		//println("starting election timeout of ", time_to_wait, me)
		time.Sleep(time.Duration(time_to_wait) * time.Millisecond)

		rf.mu.Lock()
		time_since_hb := time.Since(rf.LastHeartbeat)
		isLeader = rf.IsLeader
		rf.mu.Unlock()
		if time_since_hb > (500*time.Millisecond) && !isLeader { //if we haven't received a heartbeat and we are not the leader start election
			rf.mu.Lock()
			rf.CurrentTerm += 1
			//println("Starting new election", me, "term:", rf.CurrentTerm)
			rf.VotedFor = rf.me
			var args RequestVoteArgs
			args.CandidateID = rf.me
			args.Term = rf.CurrentTerm
			args.LastLogIndex = rf.Log[len(rf.Log)-1].Index
			args.LastLogTerm = rf.Log[len(rf.Log)-1].Term
			rf.VotesReceived = 1
			rf.mu.Unlock()
			go rf.persist()

			for i := 0; i < num_peers; i++ {
				if i != me {
					//println(me, "requesting vote from ", i)
					var reply RequestVoteReply
					go rf.sendRequestVote(i, &args, &reply)
				}
			}
			go rf.count_votes()
		}

	}
}

func (rf *Raft) count_votes() {
	rf.mu.Lock()
	total_peers := len(rf.peers)
	votes_needed := (total_peers / 2) + 1
	var current_votes int
	rf.mu.Unlock()
	time_to_win := (time.Duration(rand.Intn(100)+500) * time.Millisecond)
	time_to_win_timeout := time.Now()
	for {
		if time.Since(time_to_win_timeout) >= time_to_win { //if not within election time break
			//rf.mu.Lock()
			//println(rf.me, "election failed. did not get necessary votes in time.")
			//rf.mu.Unlock()
			break
		} else {
			rf.mu.Lock()
			current_votes = rf.VotesReceived
			rf.mu.Unlock()
			if current_votes >= votes_needed {
				rf.mu.Lock()
				rf.IsLeader = true
				rf.MatchIndex = make([]int, len(rf.peers))
				rf.NextIndex = make([]int, len(rf.peers))
				for i := 0; i < len(rf.peers); i++ {
					if i != rf.me {
						rf.NextIndex[i] = rf.Log[len(rf.Log)-1].Index
						rf.MatchIndex[i] = 0
					}
				}
				rf.mu.Unlock()
				go rf.remain_leader()
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (rf *Raft) remain_leader() {
	rf.mu.Lock()
	leaderTerm := rf.CurrentTerm
	me := rf.me
	rf.mu.Unlock()
	//println("remaining leader", me)
	for {
		this_term, is_leader := rf.GetState()
		rf.mu.Lock()
		num_peers := len(rf.peers)
		rf.mu.Unlock()
		if is_leader && this_term == leaderTerm { //if still leader, send append entries
			//println("sending heartbeats term:", this_term)
			for i := 0; i < num_peers; i++ {
				if i != me {
					rf.mu.Lock()
					var victory_args AppendEntriesArgs
					victory_args.LeaderID = rf.me
					victory_args.Term = rf.CurrentTerm
					victory_args.Entries = make([]LogEntry, 0)
					victory_args.PrevLogIndex = len(rf.Log) - 1
					victory_args.PrevLogTerm = rf.Log[len(rf.Log)-1].Term
					victory_args.LeaderCommit = rf.CommitIndex
					rf.mu.Unlock()
					var reply AppendEntriesReply
					go rf.sendAppendEntries(i, &victory_args, &reply)
				}
			}

			//println("sleeping")
			time.Sleep(250 * time.Millisecond) //send at most half of your timeout
		} else {
			return
		}

	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	//update for state variables that we add each time
	//rf.readPersist(rf.persister.raftstate) //a little confused on how this will work...
	rf.CurrentTerm = 0
	rf.VotedFor = -1
	rf.CommitIndex = 0
	rf.LastApplied = 0
	rf.IsLeader = false
	rf.VotesReceived = 0
	rf.applyChannel = applyCh
	rf.Log = make([]LogEntry, 1)
	//println("creating peer", rf.me)
	var dummyLogEntry LogEntry
	dummyLogEntry.Index = 0
	dummyLogEntry.Term = 0
	rf.Log[0] = dummyLogEntry
	var dummyMsg ApplyMsg
	dummyMsg.CommandIndex = 0
	dummyMsg.CommandValid = true
	dummyMsg.Command = 0
	rf.applyChannel <- dummyMsg

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.ticker()

	return rf
}
