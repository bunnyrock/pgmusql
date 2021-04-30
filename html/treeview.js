// treeview folders handler
function TreeViewHandler(element){
    element.parentElement.querySelector(".nested").classList.toggle("active");
    element.classList.toggle("caret-down");
}

// description handler
function DescriptionHandler(id, element) {
    var descrNodes = document.getElementsByClassName("description");
    for (var i = 0; i < descrNodes.length; i++) {
        descrNodes[i].style = "display: none;";

        if (descrNodes[i].id === id) {
            descrNodes[i].style = "";
        }
    }

    // hightlight selected item
    var itemNodes = document.getElementsByClassName("item");
    for (var i = 0; i < itemNodes.length; i++) {
        itemNodes[i].style.background = "";
        itemNodes[i].style.color = "";
    }

    element.style.background = "#008000";
    element.style.color = "#ffffff";
}

// chekbox filter handler
function filterCheckboxHandler(){
    var warnChecked = document.getElementById("WarnCheckBox").checked
    var errChecked = document.getElementById("ErrCheckBox").checked
    var searchName = document.getElementById("SearchInputBox").value.trim().toLowerCase()

    // check
    var folders = document.querySelectorAll(".container, .root");
    for(var i = 0; i < folders.length; i++) {
        
        // proc items
        var items = folders[i].querySelectorAll(".item");
        var found = false;

        for(var j = 0; j < items.length; j++){
            if (warnChecked || errChecked || searchName !== "") {
                items[j].parentElement.classList.add("filterHider");

                if(
                    ((items[j].classList.contains("haswarn") && warnChecked) || !warnChecked) && 
                    ((items[j].classList.contains("haserr") && errChecked) || !errChecked) &&
                    ((items[j].innerHTML.toLowerCase().indexOf(searchName) !== -1 && searchName !== "") || searchName == "")
                ) {
                    items[j].parentElement.classList.remove("filterHider");
                    found = true;
                }

                continue;
            }

            items[j].parentElement.classList.remove("filterHider");
        }

        if(!found && (warnChecked || errChecked || searchName !== "")){
            folders[i].classList.add("filterHider");
            continue;
        }
        folders[i].classList.remove("filterHider");
    }
}
