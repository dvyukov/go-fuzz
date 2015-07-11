package main

const statsPage = `<!DOCTYPE html>
<html>
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/css/bootstrap.min.css">
<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/css/bootstrap-theme.min.css">

<body>
  <div class="container-fluid">
    <div class="row">
      <div class="col-sm-12 col-md-12 main">
        <h1 class="page-header">Go Fuzz</h1>
        <div class="row placeholders">
          <div class="col-xs-3 col-sm-1 placeholder">
            <h4 id="slaves"></h4>
            <span class="text-muted">Slaves</span>
          </div>
          <div class="col-xs-3 col-sm-1 placeholder">
            <h4 id="corpus"></h4>
            <span class="text-muted">Corpus</span>
          </div>
          <div class="col-xs-3 col-sm-1 placeholder">
            <h4 id="crashers"></h4>
            <span class="text-muted">Crashers</span>
          </div>
          <div class="col-xs-3 col-sm-1 placeholder">
            <h4 id="restarts"></h4>
            <span class="text-muted">Restarts</span>
          </div>
          <div class="col-xs-4 col-sm-1 placeholder">
            <h4 id="execs"></h4>
            <span class="text-muted">Execs</span>
          </div>
          <div class="col-xs-4 col-sm-1 placeholder">
            <h4 id="cover"></h4>
            <span class="text-muted">Cover</span>
          </div>
          <div class="col-xs-4 col-sm-1 placeholder">
            <h4 id="uptime"></h4>
            <span class="text-muted">Uptime</span>
          </div>
        </div>

        <h2 class="sub-header">History</h2>
        <div class="table-responsive">
          <table class="table table-striped">
            <thead>
              <tr>
                <th>Slaves</th>
                <th>Corpus</th>
                <th>Crashers</th>
                <th>Restarts</th>
                <th>Execs</th>
                <th>Cover</th>
                <th>Uptime</th>
              </tr>
            </thead>
	    <tbody></tbody>
	  </table>
        </div>
      </div>
    </div>
  </div>
</div>


<script src="https://ajax.googleapis.com/ajax/libs/jquery/1.11.3/jquery.min.js"></script>
<script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.5/js/bootstrap.min.js"></script>
<script src="../../assets/js/ie10-viewport-bug-workaround.js"></script>
<script>
// First, checks if it isn't implemented yet.
if (!String.prototype.format) {
	String.prototype.format = function() {
		var args = arguments;
		return this.replace(/{(\d+)}/g, function(match, number) { 
			return typeof args[number] != 'undefined'
			? args[number]
			: match
			;
		});
	};
}

var rowFmt = "<tr><td>{0}</td><td>{1}</td><td>{2}</td><td>{3}</td><td>{4}</td><td>{5}</td><td>{6}</td></tr>"

var evtSource = new EventSource("/eventsource");
evtSource.addEventListener("ping", function(e) {
	var data = JSON.parse(e.data);
	var uptime = formatDuration(Date.now() - Date.parse(data.StartTime))
	$("tbody").prepend(rowFmt.format(
		data.Slaves,
		data.Corpus,
		data.Crashers,
		"1/" + data.RestartsDenom,
		data.Execs,
		data.Cover,
		uptime
	));

	$("#slaves").text(data.Slaves)
	$("#corpus").text(data.Corpus)
	$("#crashers").text(data.Crashers)
	$("#restarts").text("1/" + data.RestartsDenom)
	$("#execs").text(data.Execs)
	$("#cover").text(data.Cover)
	$("#uptime").text(uptime)
});

function formatDuration(ms) {
	str = "";

	days = Math.floor(ms / 1000 / 60 / 60 / 24)
	ms -=  days * 100 * 60 * 60 * 24

	if (days > 0) {
		str += days + "d "
	}

	hrs = Math.floor(ms / 1000 / 60 / 60)
	ms -= hrs * 100 * 60 * 60

	if (hrs > 0) {
		str += hrs + "h "
	}

	min = Math.floor(ms / 1000 / 60);
	ms -= min * 60 * 100;

	if (min > 0) {
		str += min + "m "
	}

	s = Math.floor(ms / 1000);
	str += s + "s"

	return str;
}

</script>
</body>
</html>
`
