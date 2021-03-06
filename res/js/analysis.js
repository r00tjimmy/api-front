
var allow_receive_req=true

socket.on("connect",function(msg){
	console && console.log("socket.io connect",msg)
	socket.emit("http_analysis",api_man_apiName)
})

socket.on("req",function(req){
	if(!allow_receive_req){
		console&&console.log("receive and skiped")
		return
	}
	console && console.log(req)
	if(req && typeof req =="object"){
		try{
			showReqDetail(req)
		}catch(e){
			console && console.log("showReqDetail err:",e)
		}
	}
})
var req_max_length=500;
var req_list=[];
var localStrName="reqs_"+api_man_apiName
try{
	if(window.localStorage && window.localStorage[localStrName]){
		req_list=$.parseJSON(window.localStorage[localStrName]||"[]")
	}
}catch(e){
	console&& console.log(e)
}

function req_clean(){
	req_list=[]
	$("#req_list").empty()
}

$().ready(function(){
	for(var i=0;i<req_list.length;i++){
		showReqTr(req_list[i])
	}
})

function showReqTr(req){
	
	var tr="<tr class='req_tr' data-reqid='"+req.id+"'>" +
			"<td>"+req.id+"</td>" +
			"<td>"+req.data.method+"</td>" +
			"<td>"+h(req.data["path"]||"unknow")+"</td>" +
			"<td>"+h(req.data["resp_status"]||502)+"</td>" +
			"<td>"+req.data.remote+"</td>"+
			"<td>"+h(req.data.master)+"</td>"+
			"<td title='ms'>"+(req.data.used && req.data.used.toFixed(2))+"</td>"
			"</tr>";
	tr+="<tr class='hidden'><td colspan=7>" +
			"<pre>"+h(formatReqData(req.data["req_detail"]||"",req.data["path"]||""))+"</pre>" +
			"<pre>"+h(showDumpData(req.data["res_detail"]||""))+"</pre>" +
			"</td>" +
			"</tr>"
	$("#req_list").prepend(tr)
	
	$("#req_list tr.req_tr").each(function(index,data){
		if(index>=req_max_length){
			data.next("tr").remove();
			data.remove();
		}
	})
}

function formatReqData(str,path){
	str+=""
	if(str.length==0){
		return str
	}
	var pos=str.indexOf("\r\n\r\n");
	var hd=str.substr(0,pos+4)+""
	var bd=str.substr(pos+4)+""
	var result=str

	var isForm=hd.indexOf("x-www-form-urlencoded")>0
	var line="<----------------------------------\n"
	var jsonBd=parseAsjson(bd)
	var pos_query=path.indexOf("?")
	if(pos_query && pos_query>0){
		var query=path.substr(pos_query+1)+""
		if(query!=""){
			result+="\n<--------GET-Params---format-------\n"
			var arr=query.split("&")
			for(var i=0;i<arr.length;i++){
				result+=urldecode(arr[i])+"\n"
			}
		}
		
		
	}
	
	var bodyFormat=""
	if(jsonBd!=false){
		bodyFormat=jsonBd
	}else if(isForm){
		var arr=bd.split("&")
		for(var i=0;i<arr.length;i++){
			var item=arr[i].split("=")
			var k=item[0],v=urldecode(item[1]||"")
			bodyFormat+=(i+1)+" ) "+k+" : "+v+"\n";
			var vjosn=parseAsjson(v)
			if(false!=vjosn){
				bodyFormat+=line+k+"_json_indent : \n"+vjosn+"\n"+line;
			}
		}
	}
	if(bodyFormat.length>0){
		result+="\n<--------body---format------------------\n"
		result+=bodyFormat
	}
	
	
	return result
}

function showDumpData(str){
	var pos=str.indexOf("\r\n\r\n");
	var hd=str.substr(0,pos+4)
	var bd=str.substr(pos+4)
	var jsonBd=parseAsjson(bd)
	if(jsonBd!=false){
		str+="\n<---------body---format------------------\n"+jsonBd
	}
	return str
}

function parseAsjson(str) {
	if(typeof str!="string"){
		return false
	}
	if(str.length<2){
		return false
	}
    try {
    	if(str[0]!="{" && str[0]!="["){
    		return false;
    	}
        var jsonObj = JSON.parse(str);
        if (jsonObj) {
        	jsonObj=revParseJson(jsonObj)
           return JSON.stringify(jsonObj, null, 4);
        }
    } catch (e) {
    	console.log("parseAsjson_error",e)
    }
    return false;
}

function revParseJson(obj){
	var t=typeof obj
	if(!$.isArray(obj) && t!="object"){
		return obj
	}
	var objNew=$.isArray(obj)?[]:{}
	$.each(obj,function(k,v){
		objNew[k]=revParseJson(v)
		if(typeof v=="string" && v.length>2 && (v[0]=="["||v[0]=="{")){
			try{
				var tmp=JSON.parse(v);
				if(tmp!=false){
					objNew[k+"_json_decode"]=tmp
				}
			}catch(e){
			}
		}
	})
	return objNew
}
	
function showReqDetail(req){
	if(req && req.data){
	   req.data.req_detail=base64_decode(req.data.req_detail)
	   req.data.res_detail=base64_decode(req.data.res_detail)
	}
	showReqTr(req)
	req_list.push(req)
	while(req_max_length>0 && req_list.length>req_max_length){
		req_list.shift();
	}
}

window.onbeforeunload=function(){
	if(req_max_length>0){
		window.localStorage[localStrName]=JSON.stringify(req_list)
	}
}

$().ready(function(){
	$("#req_list").on("click","tr.req_tr",function(){
		$(this).next("tr").toggleClass("hidden");
		location.hash=$(this).data("reqid")+""
	})
	$("#item_open_all").click(function(){
		$("#req_list tr").not(".req_tr").removeClass("hidden")
		return false;
	});
	$("#item_close_all").click(function(){
		$("#req_list tr").not(".req_tr").addClass("hidden")
		return false;
	});
	$("#item_checkbox_receive").click(function(){
		allow_receive_req=$(this).is(":checked")
	});
	
	if(location.hash!="" && location.hash.length>8){
		$("#req_list tr.req_tr").each(function(){
			if("#"+$(this).data("reqid")==location.hash){
				$(this).next("tr").toggleClass("hidden");
			}
		});
	}
});