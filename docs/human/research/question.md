## agents

### sink

```js
const chatWriter: SessionSink['write'] = async (event, data) => {
lastWriteAt = Date.now()
await stream.writeSSE({ event, data })
}
const sink: SessionSink = {
write: chatWriter,
buffer: [],
disconnected: false,
doneResolve,
donePromise,
}
```

`sink.write('message', data)` 的时候没有作异常处理，不会重新放回到 buffer 中。是不是可以理解成，这里不保证至少一次投递的寓意。
