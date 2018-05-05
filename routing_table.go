package kademlia

import (
	"container/list"
	"crypto/sha1"
	"math/big"
	"net"
	"sort"
	"sync"
)

// Contact is an entry in the k-bucket
type Contact struct {
	Id   big.Int
	Addr net.TCPAddr
}

// NewContact creates a new Contact struct based on addr by taking the hash
func NewContact(addr net.TCPAddr) *Contact {
	hash := sha1.Sum([]byte(addr.String()))

	id := *big.NewInt(0)
	id.SetBytes(hash[:])

	nodeEntry := Contact{id, addr}
	return &nodeEntry
}

// AreEqualContacts returns true if contact Id and Addr are equivalent
// structs can be compared, but structs containing big.Int cannot
func AreEqualContacts(a *Contact, b *Contact) bool {
	return (a.Id.Cmp(&b.Id) == 0)
}

// extra struct because we will want to implement split bucket
type RoutingTable struct {
	owner        *Node
	kBuckets     []*KBucket
	numNeighbors int
	mu           *sync.Mutex
}

func NewRoutingTable(owner *Node) *RoutingTable {
	kBuckets := make([]*KBucket, 160)
	numNeighbors := 0
	mu := &sync.Mutex{}
	rt := RoutingTable{owner, kBuckets, numNeighbors, mu}
	return &rt
}

// section 2.4 Kademlia protocol splits bucket when full and range includes own ID
// TODO
func (self *RoutingTable) splitBucket() {

}

func (self *RoutingTable) findKNearestContacts(id big.Int) []Contact {
	// If the entire RT has less than k contacts, then just return all the contacts

	kNearest := make([]Contact, k)
	// To find the k closest contacts, we start looking from the bucket that the contact would be in
	index := self.owner.GetKBucketFromID(&id)
	copy(kNearest, self.kBuckets[index].getAllContacts())

	// If less than k contacts are in the bucket, then take the closest from the left
	if len(kNearest) < k {
		// 0th bucket never populated
		for curr := index - 1; curr > 0; index-- {
			currBucket := self.kBuckets[curr]
			kNearest = append(kNearest, currBucket.getAllContacts()...)
			if len(kNearest) >= k {
				break
			}
		}
	}

	// Then go to the right
	if len(kNearest) < k {
		for curr := index + 1; curr < len(self.kBuckets); curr++ {
			currBucket := self.kBuckets[curr]
			kNearest = append(kNearest, currBucket.getAllContacts()...)
			if len(kNearest) >= k {
				break
			}
		}
	}

	// Return in order of distance to contact
	kNearest = kNearest[:k]
	sort.Slice(kNearest, func(i, j int) bool {
		aDist := float64(distanceBetween(id, kNearest[i].Id).Uint64())
		bDist := float64(distanceBetween(id, kNearest[j].Id).Uint64())
		return aDist < bDist
	})

	return kNearest
}

func (self *RoutingTable) add(contact Contact) {
	index := self.owner.GetKBucketFromAddr(contact.Addr)
	self.owner.logger.Printf("Adding node to bucket %d", index)
	if self.kBuckets[index] == nil {
		self.owner.logger.Printf("Creating bucket %d", index)
		self.kBuckets[index] = NewKBucket(20)
	}
	self.kBuckets[index].addContact(contact)
	self.owner.logger.Printf("Bucket after add: %v", self.kBuckets[index].contacts)
	//TODO: handle failure to add
}

func (self *RoutingTable) remove(contact Contact) {
	index := self.owner.GetKBucketFromAddr(contact.Addr)
	self.kBuckets[index].removeContact(contact)
}

// Not even sure if we will use this
func (self *RoutingTable) clear() {
	// Note that this sets slice capacity to 0
	self.kBuckets = nil
}

type KBucket struct {
	contacts list.List
	k        int       // max number of contacts
	lruCache list.List // not implemented yet but explained in section 4.1
}

func NewKBucket(k int) *KBucket {
	contacts := *list.New()
	lruCache := *list.New()
	kBucket := KBucket{contacts, k, lruCache}
	return &kBucket
}

// If bucket contains contact, returns ptr to element in list. Else, returns nil
func (self *KBucket) getFromList(contact Contact) *list.Element {
	for e := self.contacts.Front(); e != nil; e = e.Next() {
		curr, _ := e.Value.(Contact)
		// TODO: handle error when element can't be cast to Contact
		if AreEqualContacts(&curr, &contact) {
			return e
		}
	}
	return nil
}

// Not nice, but need this functionality because contacts are a list
func (self *KBucket) getAllContacts() []Contact {
	contacts := make([]Contact, 20)
	index := 0
	for e := self.contacts.Front(); e != nil; e = e.Next() {
		curr, _ := e.Value.(Contact)
		contacts[index] = curr
		index++
	}
	return contacts
}

// Returns true if contact is added into bucket, false otherwise
func (self *KBucket) addContact(contact Contact) bool {
	// If contact exists, move to tail
	element := self.getFromList(contact)
	if element != nil {
		self.contacts.MoveToFront(element)
		return true
	} else {
		// If bucket isn't full, add to tail
		// list.Len() = O(1)
		if self.contacts.Len() < self.k {
			self.contacts.PushFront(contact)
			return true
		}
		/* TODO: Deal when with buckets are full
		// Otherwise, ping least-recently seen node
		lruNode := self.contacts.Front()
		// ping node... sigh this is gnna be ugly.
		if true {
			// If no response, node is evicted and new sender is inserted at tail
			self.contacts.Remove(lruNode)
			self.contacts.PushBack(contact)
			return true
		}
		// implement replacement cache
		return false
		*/
		return false
	}
}

// ContactFromID returns the contact that belongs to id if it exists and nil if
// it doesn't
func (table *RoutingTable) ContactFromID(id big.Int) *Contact {
	contact := Contact{id, net.TCPAddr{}}

	// find the bucket it should be in
	// if the bucket has been allocated (isn't nil), see if it's
	// in the list

	index := table.owner.GetKBucketFromID(&id)
	table.owner.logger.Printf("Index is %d", index)
	kbucket := table.kBuckets[index]

	if kbucket != nil {
		table.owner.logger.Printf("Found a kbucket")
		result := kbucket.getFromList(contact)
		if result != nil {
			toReturn := result.Value.(Contact)
			return &toReturn
		}
	} else {
		return nil
	}
	return nil
}

// Returns true if contact exists, false otherwise
func (self *KBucket) removeContact(contact Contact) bool {
	element := self.getFromList(contact)
	if element != nil {
		self.contacts.Remove(element)
		return true
	} else {
		return false
	}
}