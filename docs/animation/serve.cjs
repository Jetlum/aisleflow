// Local-only preview. Only presentation assets can be served.
const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assets = new Map([
  ['/', ['index.html', 'text/html; charset=utf-8']],
  ['/index.html', ['index.html', 'text/html; charset=utf-8']],
  ['/style.css', ['style.css', 'text/css; charset=utf-8']],
  ['/animation.js', ['animation.js', 'text/javascript; charset=utf-8']],
  ['/story.js', ['story.js', 'text/javascript; charset=utf-8']],
  ['/assets/pyck-logo.svg', ['assets/pyck-logo.svg', 'image/svg+xml']],
  ['/assets/montserrat-latin.woff2', ['assets/montserrat-latin.woff2', 'font/woff2']]
]);
const port = Number(process.env.PYCK_ANIMATION_PORT || 4173);
const server = http.createServer((req,res)=>{
  if(!['GET','HEAD'].includes(req.method)){res.writeHead(405);res.end();return;}
  const asset=assets.get(req.url.split('?')[0]);
  if(!asset){res.writeHead(404);res.end('Not found');return;}
  fs.readFile(path.join(__dirname,asset[0]),(error,bytes)=>{
    if(error){res.writeHead(500);res.end('Asset unavailable');return;}
    res.writeHead(200,{'Content-Type':asset[1],'Cache-Control':'no-store','X-Content-Type-Options':'nosniff'});
    res.end(req.method==='HEAD'?undefined:bytes);
  });
});
server.on('error',error=>{console.error(error.message);process.exitCode=1;});
server.listen(port,'127.0.0.1',()=>console.log(`Pyck animation: http://localhost:${port}`));
