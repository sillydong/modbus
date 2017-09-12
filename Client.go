package modbus

import (
	"log"
	"sync"
	"time"
)

// ConnectionSettings holds all connection settings. Connections and clients
// are uniquely identified by their Host string. For ModeTCP this is the FQDN
// or IP address AND the port number. For ModeRTU and ModeASCII the Host string
// holds the full path to the serial device (Linux) or the name of the COM port
// (Windows).
type ConnectionSettings struct {
	Mode
	Host    string
	Baud    int
	Timeout time.Duration
}

// Client contains the connection settings, the connection handler, and the qq
// used to listen for queries.
type Client struct {
	isManagedClient bool
	ConnectionSettings
	Packager

	mu sync.Mutex
	wg sync.WaitGroup

	qq          chan Query
	newQQSignal chan interface{}
}

// queryListener executes Queries sent on the qq and sends QueryResponses to
// the Query's Response channel.
func (c *Client) queryListener() {
	defer c.Close()
	// Set up connection for slave
	for qry := range c.qq {
		qry := qry
		if nil == qry.Response {
			log.Println("No Query.Response channel set up")
			continue
		}
		c.SetQuery(qry)
		res, err := c.Send()
		if nil != err {
			go qry.sendResponse(nil, err)
			continue
		}
		go qry.sendResponse(res, nil)
	}
}

// start sets up the appropriate transporter and packager and if
// successful, creates the qq channel and starts the Client's goroutine.
func (c *Client) Start() (chan Query, error) {
	switch c.Mode {
	case ModeTCP:
		p, err := NewTCPPackager(c.ConnectionSettings)
		if nil != err {
			return nil, err
		}
		c.Packager = p
	case ModeRTU:
		p, err := NewRTUPackager(c.ConnectionSettings)
		if nil != err {
			return nil, err
		}
		c.Packager = p
	case ModeASCII:
		p, err := NewASCIIPackager(c.ConnectionSettings)
		if nil != err {
			return nil, err
		}
		c.Packager = p
	}

	c.qq = make(chan Query)
	c.newQQSignal = make(chan interface{})
	go c.queryListener()

	qq := c.newQueryQueue()
	go func() {
		var run = true
		for run {
			c.wg.Wait()
			c.mu.Lock()
			select {
			case <-c.newQQSignal:
				go func() {
					c.newQQSignal <- true
				}()
				c.mu.Unlock()
				continue
			default:
				run = false
			}
		}
		if c.isManagedClient {
			clntMngr.deleteClient <- &c.Host
		}
		close(c.qq)
		close(c.newQQSignal)
		c.qq = nil
		c.newQQSignal = nil
		c.mu.Unlock()
	}()
	return qq, nil
}

func (c *Client) queryQueueChannelMonitor() {
	var run = true
	for run {
		// Wait until all QueryQueue channels have signaled Done()
		c.wg.Wait()
		c.mu.Lock()
		// This is a check for any QueryQueue channels that may have been created
		// between Wait() returning and acquiring the Lock().
		select {
		case <-c.newQQSignal:
			// Relaunch the goroutine holding the blocking newQQSignal signal
			go func() {
				c.newQQSignal <- true
			}()
			c.mu.Unlock()
			continue
		default:
			run = false
		}
	}
	if c.isManagedClient {
		clntMngr.deleteClient <- &c.Host
	}
	close(c.qq)
	close(c.newQQSignal)
	c.qq = nil
	c.newQQSignal = nil
	c.mu.Unlock()
}

// newQueryQueue generates a new QueryQueue channel and a goroutine that
// forwards the queries onto the connection's main internal qq. Each goroutine that
// sends queries to the connection needs their own QueryQueue if they are to be
// allowed to close the channel. connections with no remaining open channels shut
// themselves down.
func (c *Client) newQueryQueue() chan Query {
	if nil == c.qq {
		log.Fatal("Client is not running")
	}
	// This watch group tracks the number of open channels
	c.wg.Add(1)
	qq := make(chan Query)

	// This goroutine sends a blocking newQQSignal which is cleared when
	// the forwarding goroutine exits on channel close. This allows the
	// queryQueueChannelMonitor to avoid a race condition between shutting
	// down the connection due to all channels closing and another goroutine,
	// such as the the ClientManager's requestListener, setting up a new
	// Query channel.
	go func() {
		c.newQQSignal <- true
	}()
	// This goroutine forwards queries from the newly created QueryQueue
	// onto the connection's main internal qq.
	go func() {
		for q := range qq {
			//log.Println(q)
			c.qq <- q
		}
		<-c.newQQSignal // Consume newQQSignal before signaling Done()
		c.wg.Done()
	}()
	return qq
}
