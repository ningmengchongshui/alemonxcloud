async page => {
 const instance = { id: 'demo-instance', name: '演示实例', image: 'app/demo', version: 'latest', spec: '2 核 / 1 GB', status: 'running', runtimeStatus: 'running', containerName: 'xcloud-0123456789abcdef', ip: 'https://example.com', createdAt: '2026-11-06T00:00:00Z', hostAccessAllowed: true, hostAccessEnabled: false, hostAccessApplied: false };
 await page.route('**/api/**', async route => {
 const path = new URL(route.request().url()).pathname;
 let body = [];
 if(path === '/api/oidc/session') body = {user:{id:'demo',username:'本地测试',isAdmin:false}};
 else if(path === '/api/instances') body = [instance];
 else if(path.endsWith('/summary')) body = {instance,selfHosted:false,nodeId:'node-demo'};
 else if(path === '/api/instances/demo-instance') body = {metadata:{id:instance.id,resourceVersion:1},spec:{},status:{lifecycle:'running',runtimeState:'running',observedCpu:2,observedMemoryMB:1024,observedGeneration:1},conditions:[],operations:[]};
 else if(path.endsWith('/stats')) body = {cpuPercent:'125.50%',memoryUsage:'256MiB / 1GiB',memoryPercent:'25.00%',networkIO:'10MB / 20MB',blockIO:'1MB / 2MB',pids:'8',sampledAt:new Date().toISOString()};
 else if(path==='/api/catalog') body={plans:[],images:[]};
 else if(path==='/api/wallet') body={balanceFen:0};
 await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(body)});
 });
 await page.goto('http://127.0.0.1:5173/me/instances');
}
