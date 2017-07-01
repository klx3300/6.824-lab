package mapreduce

import (
	"container/list"
	"encoding/json"
	"fmt"
	"os"
)

// doReduce does the job of a reduce worker: it reads the intermediate
// key/value pairs (produced by the map phase) for this task, sorts the
// intermediate key/value pairs by key, calls the user-defined reduce function
// (reduceF) for each key, and writes the output to disk.
func doReduce(
	jobName string, // the name of the whole MapReduce job
	reduceTaskNumber int, // which reduce task this is
	nMap int, // the number of map tasks that were run ("M" in the paper)
	reduceF func(key string, values []string) string,
) {
	// TODO:
	// You will need to write this function.
	// You can find the intermediate file for this reduce task from map task number
	// m using reduceName(jobName, m, reduceTaskNumber).
	// Remember that you've encoded the values in the intermediate files, so you
	// will need to decode them. If you chose to use JSON, you can read out
	// multiple decoded values by creating a decoder, and then repeatedly calling
	// .Decode() on it until Decode() returns an error.
	//
	// You should write the reduced output in as JSON encoded KeyValue
	// objects to a file named mergeName(jobName, reduceTaskNumber). We require
	// you to use JSON here because that is what the merger than combines the
	// output from all the reduce tasks expects. There is nothing "special" about
	// JSON -- it is just the marshalling format we chose to use. It will look
	// something like this:
	//
	// enc := json.NewEncoder(mergeFile)
	// for key in ... {
	// 	enc.Encode(KeyValue{key, reduceF(...)})
	// }
	// file.Close()
	rFiles := make([]*os.File, nMap)
	rDecoders := make([]*json.Decoder, nMap)
	for i := 0; i < nMap; i++ {
		rFile, rErr := os.Open(reduceName(jobName, i, reduceTaskNumber))
		if rErr != nil {
			// there are many things can go wrong in the world
			// we just yell about it and continues our trip
			fmt.Printf("Error occurred at i/o-readMapped: %s", rErr.Error())
		}
		rFiles[i] = rFile
		defer rFiles[i].Close()
		rDecoders[i] = json.NewDecoder(rFiles[i])
	}
	mFile, mErr := os.OpenFile(mergeName(jobName, reduceTaskNumber), os.O_WRONLY|os.O_CREATE, 0666)
	if mErr != nil {
		// your disks must have much problem waiting to fix!
		fmt.Printf("Error occurred at i/0-openReduced: %s\n", mErr.Error())
	}
	defer mFile.Close()

	mEncoder := json.NewEncoder(mFile)
	mapKeys := make(map[string]*list.List)
	for fIt := 0; fIt < nMap; fIt++ {
		for {
			var tmpkv KeyValue
			rDecodeErr := rDecoders[fIt].Decode(&tmpkv)
			if rDecodeErr != nil {
				// that must be eof caused that. let's quit the loop
				break
			}
			// is previously existed in the map?
			tmpV, tmpExist := mapKeys[tmpkv.Key]
			if tmpExist {
				tmpV.PushBack(tmpkv.Value)
			} else {
				mapKeys[tmpkv.Key] = list.New()
				mapKeys[tmpkv.Key].PushBack(tmpkv.Value)
			}
		}
		// // something must be done at last
		// tmparr := make([]string, specKeyList.Len())

		// for listIt, listItCnt := specKeyList.Front(), 0; listIt != nil; listIt, listItCnt = listIt.Next(), listItCnt+1 {
		// 	tmpConvVal, tmpConvIsCorrect := listIt.Value.(KeyValue)
		// 	if tmpConvIsCorrect {
		// 		tmparr[listItCnt] = tmpConvVal.Value
		// 	}
		// }
		// // send this array to reduce func
		// rRet := reduceF(lastKey, tmparr)
		// tmpToEnc := KeyValue{lastKey, rRet}
		// fmt.Println("Reduce func encoded", tmpToEnc)
		// mEncoder.Encode(tmpToEnc)
	}
	// for each elem in map, conv them to array and send to reduceF
	for tmpK, tmpLst := range mapKeys {
		tmpArr := make([]string, tmpLst.Len())
		for listIt, listItCnt := tmpLst.Front(), 0; listIt != nil; listIt, listItCnt = listIt.Next(), listItCnt+1 {
			tmpConvV, tmpConvCorr := listIt.Value.(string)
			if tmpConvCorr {
				tmpArr[listItCnt] = tmpConvV
			} else {
				fmt.Println("Unexpected error occurred at doReduce func")
			}
		}
		// send to reduceF
		tmpRet := reduceF(tmpK, tmpArr)
		mEncoder.Encode(KeyValue{tmpK, tmpRet})
	}
}
