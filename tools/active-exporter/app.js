const express = require('express');
const app = express();
let fs = require('fs');

let stat_file = process.env.STAT_FILE;
let listen_port = process.env.LISTEN_PORT;

console.log(`state file at: ${stat_file}`)

app.get('/mtime', (req, res) => {
    if (!fs.existsSync(stat_file)) {
        console.log(`${stat_file} not exists.`)
        res.status(204).send()
    } else {
        res.status(200).send(fs.statSync(stat_file).mtime);
    }
});

app.listen(listen_port, () => console.log(`active-exporter app listening on port ${listen_port}!`));
